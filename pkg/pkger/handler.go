// If you are AI: This file implements the HTTP handlers for /hls/* and /dash/*.
// Routes resolve to a Manager-owned packager and serve files from its work dir.

package pkger

import (
	"context"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Handler serves packaged HLS / DASH files via HTTP.
type Handler struct {
	mgr *Manager
}

// NewHandler binds a handler to the given manager.
func NewHandler(mgr *Manager) *Handler { return &Handler{mgr: mgr} }

// RegisterRoutes mounts /hls/ and /dash/ on the supplied mux. Both prefixes
// must be registered BEFORE the httpflv catch-all route.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/hls/", h.serveHLS)
	mux.HandleFunc("/dash/", h.serveDASH)
}

// serveHLS routes /hls/{app}/{name}.m3u8 plus its segments.
// In LL-HLS mode the segments are .m4s + a top-level init.mp4.
func (h *Handler) serveHLS(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, FormatHLS, "/hls/", []string{".m3u8", ".ts", ".m4s", ".mp4"})
}

// serveDASH routes /dash/{app}/{name}.mpd and /dash/{app}/{name}/{file}.
func (h *Handler) serveDASH(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, FormatDASH, "/dash/", []string{".mpd", ".m4s", ".mp4"})
}

// serve resolves the packager and serves the requested file from its work dir.
// It supports two URL shapes:
//
//	/hls/{app}/{name}.m3u8     (manifest at the top level)
//	/hls/{app}/{name}/{file}   (segment under the stream)
func (h *Handler) serve(w http.ResponseWriter, r *http.Request, format Format, prefix string, exts []string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		http.Error(w, "ffmpeg not available on this server", http.StatusServiceUnavailable)
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, prefix)

	// Legacy convenience URL: /hls/{app}/{name}.m3u8 (or .mpd) — redirect to
	// the canonical /hls/{app}/{name}/index.m3u8 form so that relative URI
	// resolution in the playlist points at sibling segments. Without this,
	// hls.js / dash.js fetch segments from the wrong path and fail.
	if to, ok := legacyManifestRedirect(rel, prefix, format); ok {
		http.Redirect(w, r, to, http.StatusMovedPermanently)
		return
	}

	app, name, file, _, ok := splitPath(rel, exts)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	pkg, err := h.mgr.GetOrCreate(app, name, format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	pkg.Touch()

	// Treat a request whose tail filename equals the packager's manifest
	// (e.g. "index.m3u8") as a manifest request — we wait for ffmpeg to
	// produce it before serving.
	if filepath.Base(file) == pkg.Manifest() {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := pkg.WaitReady(ctx, 15*time.Second); err != nil {
			http.Error(w, "manifest not ready: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	full := filepath.Join(pkg.WorkDir(), file)
	// Defence in depth: don't allow path traversal out of the work dir.
	if !strings.HasPrefix(full, pkg.WorkDir()+string(filepath.Separator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	// Wide-open CORS for playback. Players (hls.js, dash.js, Shaka) typically
	// pull the manifest cross-origin from a different host or port, so without
	// this they fail with a CORS preflight error.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	if strings.HasSuffix(file, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else if strings.HasSuffix(file, ".mpd") {
		w.Header().Set("Content-Type", "application/dash+xml")
	}
	http.ServeFile(w, r, full)
}

// splitPath parses URLs of the canonical form:
//
//	{app}/{name}/{file.ext}               (manifest or single-rendition segment)
//	{app}/{name}/{rendition}/{file.ext}   (per-rendition file — ABR)
//
// The trailing component's extension must be in exts. The 2-part legacy form
// (`/hls/{app}/{name}.m3u8`) is handled by legacyManifestRedirect upstream
// and never reaches here. The handler treats a request whose `file` matches
// pkg.Manifest() as a manifest request (waiting for ffmpeg to produce it).
func splitPath(rel string, exts []string) (app, name, file string, isManifest, ok bool) {
	if rel == "" || strings.Contains(rel, "..") {
		return "", "", "", false, false
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return "", "", "", false, false
	}
	last := parts[len(parts)-1]
	if !contains(exts, filepath.Ext(last)) {
		return "", "", "", false, false
	}
	for _, seg := range parts[2:] {
		if !isSafeSegment(seg) {
			return "", "", "", false, false
		}
	}
	return parts[0], parts[1], strings.Join(parts[2:], "/"), false, true
}

// isSafeSegment rejects empty, dotted-only, or slash-bearing path segments.
// Combined with the explicit ".." reject above, this keeps the segments
// inside the packager work dir.
func isSafeSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	return !strings.ContainsAny(s, `/\`)
}

// legacyManifestRedirect detects the /hls/{app}/{name}.m3u8 (or .mpd) form
// and returns the canonical /hls/{app}/{name}/index.m3u8 (or manifest.mpd)
// URL it should 301 to. Returns ("", false) if rel isn't this shape.
func legacyManifestRedirect(rel, prefix string, format Format) (string, bool) {
	if rel == "" || strings.Contains(rel, "..") {
		return "", false
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		return "", false
	}
	base := parts[1]
	ext := filepath.Ext(base)
	if ext != ".m3u8" && ext != ".mpd" {
		return "", false
	}
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		return "", false
	}
	canonical := "index.m3u8"
	if format == FormatDASH {
		canonical = "manifest.mpd"
	}
	return prefix + parts[0] + "/" + stem + "/" + canonical, true
}

// contains reports whether s is in slice.
func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}

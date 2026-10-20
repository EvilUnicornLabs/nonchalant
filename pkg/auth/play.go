// If you are AI: This file provides an HTTP middleware that enforces play-side
// pre-shared-key authentication for HTTP-FLV / WS-FLV / HLS / DASH endpoints.

package auth

import "net/http"

// Gate returns an http.Handler that enforces ks against the "key" query
// parameter before delegating to next. If ks is nil the gate is a pass-through
// (anonymous playback). On rejection the response is 401 with a short body and
// no body data is leaked from next.
func Gate(ks *KeySet, next http.Handler) http.Handler {
	if ks == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ks.Allow(r.URL.Query().Get("key")) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

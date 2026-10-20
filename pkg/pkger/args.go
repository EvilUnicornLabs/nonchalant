// If you are AI: This file builds the ffmpeg command line for one Packager.
// Single-rendition mode does stream-copy (cheap). ABR mode (Options.Ladder
// non-empty) transcodes one rendition per rung.

package pkger

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ffmpegArgs builds the format-specific ffmpeg command line for the packager.
// Returns the args list to pass to exec.Command. Behaviour fans out on
// (format, opts.Ladder, opts.LowLatency).
func (p *Packager) ffmpegArgs() []string {
	common := []string{
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+nobuffer",
		"-i", p.sourceURL,
	}
	if len(p.opts.Ladder) > 0 {
		return p.abrArgs(common)
	}
	return p.singleRenditionArgs(common)
}

// singleRenditionArgs returns the original stream-copy invocation.
// Cheap — no transcoding — but produces only one variant.
func (p *Packager) singleRenditionArgs(common []string) []string {
	args := append([]string{}, common...)
	args = append(args, "-c", "copy")
	manifestPath := filepath.Join(p.workDir, p.manifest)
	switch p.format {
	case FormatDASH:
		segDur := "2"
		if p.opts.LowLatency {
			segDur = "1"
		}
		return append(args,
			"-f", "dash",
			"-seg_duration", segDur,
			"-window_size", "5",
			"-remove_at_exit", "1",
			"-y", manifestPath,
		)
	}
	// HLS
	segPattern := filepath.Join(p.workDir, "seg_%05d.ts")
	hlsTime := "2"
	hlsFlags := "delete_segments+independent_segments"
	hlsArgs := []string{
		"-f", "hls",
		"-hls_list_size", "5",
	}
	if p.opts.LowLatency {
		hlsTime = "1"
		hlsFlags = "delete_segments+independent_segments+program_date_time"
		hlsArgs = append(hlsArgs, "-hls_segment_type", "fmp4")
		segPattern = filepath.Join(p.workDir, "seg_%05d.m4s")
		hlsArgs = append(hlsArgs, "-hls_fmp4_init_filename", "init.mp4")
	}
	hlsArgs = append(hlsArgs,
		"-hls_time", hlsTime,
		"-hls_flags", hlsFlags,
		"-hls_segment_filename", segPattern,
		"-y", manifestPath,
	)
	return append(args, hlsArgs...)
}

// abrArgs returns the ffmpeg invocation for ABR-mode packaging.
// Each rung becomes one libx264 video output (or shared aac audio for
// audio-only rungs). HLS uses var_stream_map; DASH uses adaptation_sets.
func (p *Packager) abrArgs(common []string) []string {
	args := append([]string{}, common...)
	video, audio := splitLadder(p.opts.Ladder)

	gop := "48" // 2 s GOP at 24 fps; matches our 2 s segments
	if p.opts.LowLatency {
		gop = "24" // 1 s GOP for 1 s segments
	}

	// Per video rung: map video AND audio. ffmpeg's HLS muxer rejects sharing
	// the same audio elementary stream across variants ("Same elementary
	// stream found more than once"), so we map audio separately for each
	// variant. AAC re-encoding is cheap; the ABR cost is dominated by H.264.
	abr := fmt.Sprintf("%dk", audioBitrate(p.opts.Ladder))
	for i, r := range video {
		args = append(args, "-map", "0:v:0", "-map", "0:a:0?",
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-preset:v:%d", i), "veryfast",
			fmt.Sprintf("-tune:v:%d", i), "zerolatency",
			fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", r.VideoBitrate),
			fmt.Sprintf("-s:v:%d", i), fmt.Sprintf("%dx%d", r.Width, r.Height),
			fmt.Sprintf("-g:v:%d", i), gop,
			fmt.Sprintf("-keyint_min:v:%d", i), gop,
			fmt.Sprintf("-c:a:%d", i), "aac",
			fmt.Sprintf("-b:a:%d", i), abr,
		)
	}

	// Audio-only rungs are appended after video. They share the same source
	// audio stream but produce a standalone variant for audio-only playback.
	for j, r := range audio {
		idx := len(video) + j
		bitrate := r.AudioBitrate
		if bitrate <= 0 {
			bitrate = 64
		}
		args = append(args, "-map", "0:a:0",
			fmt.Sprintf("-c:a:%d", idx), "aac",
			fmt.Sprintf("-b:a:%d", idx), fmt.Sprintf("%dk", bitrate),
		)
	}

	if p.format == FormatDASH {
		args = append(args, dashABRArgs(p.workDir, p.opts.LowLatency)...)
	} else {
		args = append(args, hlsABRArgs(p.workDir, p.manifest, p.opts, video, audio)...)
	}
	return args
}

// splitLadder partitions the ladder into video rungs and audio-only rungs.
// Their relative order is preserved, which determines URL layout.
func splitLadder(ladder []LadderRung) (video, audio []LadderRung) {
	for _, r := range ladder {
		if r.AudioOnly {
			audio = append(audio, r)
		} else {
			video = append(video, r)
		}
	}
	return
}

// audioBitrate picks the highest configured audio_bitrate from any rung,
// defaulting to 128 kbit/s. We transcode audio once and share it; the highest
// requested rate wins.
func audioBitrate(ladder []LadderRung) int {
	best := 128
	for _, r := range ladder {
		if r.AudioBitrate > best {
			best = r.AudioBitrate
		}
	}
	return best
}

// hlsABRArgs returns the HLS-specific tail for ABR mode: var_stream_map plus
// the per-variant playlist + segment template paths.
func hlsABRArgs(workDir, manifest string, opts Options, video, audio []LadderRung) []string {
	hlsTime := "2"
	hlsFlags := "delete_segments+independent_segments"
	if opts.LowLatency {
		hlsTime = "1"
		hlsFlags = "delete_segments+independent_segments+program_date_time"
	}
	streamMap := buildVarStreamMap(video, audio)
	args := []string{
		"-f", "hls",
		"-hls_time", hlsTime,
		"-hls_list_size", "5",
		"-hls_flags", hlsFlags,
		"-master_pl_name", manifest, // top-level master playlist filename
		"-var_stream_map", streamMap,
		"-hls_segment_filename", filepath.Join(workDir, "%v", "seg_%05d.ts"),
		"-y", filepath.Join(workDir, "%v", "index.m3u8"),
	}
	return args
}

// buildVarStreamMap assembles the ffmpeg -var_stream_map argument.
// Each video rung pairs with its own audio output (a:0, a:1, ...); audio-only
// rungs follow at indices len(video), len(video)+1, ....
// Names map directly to URL path segments under /hls/{app}/{name}/{rung}/.
func buildVarStreamMap(video, audio []LadderRung) string {
	parts := make([]string, 0, len(video)+len(audio))
	for i, r := range video {
		parts = append(parts, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, r.Name))
	}
	for j, r := range audio {
		parts = append(parts, fmt.Sprintf("a:%d,name:%s", len(video)+j, r.Name))
	}
	return strings.Join(parts, " ")
}

// dashABRArgs returns the DASH-specific tail for ABR mode. The dash muxer
// auto-creates one Representation per output stream; we just declare two
// AdaptationSets (video, audio) so DASH-IF clients see them properly.
func dashABRArgs(workDir string, lowLatency bool) []string {
	segDur := "2"
	if lowLatency {
		segDur = "1"
	}
	return []string{
		"-f", "dash",
		"-seg_duration", segDur,
		"-window_size", "5",
		"-remove_at_exit", "1",
		"-adaptation_sets", "id=0,streams=v id=1,streams=a",
		"-y", filepath.Join(workDir, "manifest.mpd"),
	}
}

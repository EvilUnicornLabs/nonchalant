// If you are AI: This file implements push relay functionality.
// Push relay shells out to ffmpeg to fetch our local HTTP-FLV stream and
// publish it to a remote RTMP server. ffmpeg handles the full RTMP client
// protocol so we don't have to.

package relay

import (
	"context"
	"fmt"

	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// PushTask implements push relay (subscribe local, publish remote).
type PushTask struct {
	*BaseTask
}

// NewPushTask creates a new push relay task.
func NewPushTask(registry *bus.Registry, app, name, remoteURL string, reconnect bool) *PushTask {
	return &PushTask{
		BaseTask: NewBaseTask(registry, app, name, remoteURL, reconnect),
	}
}

// Start runs an ffmpeg subprocess that consumes our local HTTP-FLV stream and
// publishes to the remote RTMP server. On exit it retries with backoff while
// reconnect is enabled. Returns when ctx is cancelled or the task is Stop()'d.
func (t *PushTask) Start(ctx context.Context) error {
	t.SetRunning(true)
	defer t.SetRunning(false)

	args := []string{
		"-hide_banner", "-loglevel", "warning",
		// No -re: the source is already live-paced (our own HTTP-FLV).
		// -fflags +nobuffer avoids initial demuxer buffering.
		"-fflags", "+nobuffer",
		"-i", t.localFLVSource(),
		"-c", "copy",
		"-f", "flv",
		t.RemoteURL(),
	}
	return t.runFFmpegLoop(ctx, fmt.Sprintf("push %s/%s", t.App(), t.Name()), args)
}

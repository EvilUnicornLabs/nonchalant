// If you are AI: This file implements pull relay functionality.
// Pull relay shells out to ffmpeg to fetch a remote RTMP stream and republish
// it to our own RTMP ingest. ffmpeg handles the full RTMP client protocol so
// we don't have to.

package relay

import (
	"context"
	"fmt"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// PullTask implements pull relay (connect to remote, play, republish locally).
type PullTask struct {
	*BaseTask
}

// NewPullTask creates a new pull relay task.
func NewPullTask(registry *bus.Registry, app, name, remoteURL string, reconnect bool) *PullTask {
	return &PullTask{
		BaseTask: NewBaseTask(registry, app, name, remoteURL, reconnect),
	}
}

// Start runs an ffmpeg subprocess that pulls from the remote RTMP server and
// republishes to our local RTMP ingest. On exit it retries with exponential
// backoff while reconnect is enabled. Returns when ctx is cancelled or the
// task is Stop()'d.
func (t *PullTask) Start(ctx context.Context) error {
	t.SetRunning(true)
	defer t.SetRunning(false)

	args := []string{
		"-hide_banner", "-loglevel", "warning",
		"-i", t.RemoteURL(),
		"-c", "copy",
		"-f", "flv",
		t.localRTMPTarget(),
	}
	return t.runFFmpegLoop(ctx, fmt.Sprintf("pull %s/%s", t.App(), t.Name()), args)
}

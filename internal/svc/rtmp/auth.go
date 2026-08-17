// If you are AI: This file implements publish-side authentication for RTMP.
// Authentication is shared-secret based: clients pass "?key=<secret>" in the stream name.

package rtmp

import (
	"net/url"
	"strings"

	"github.com/vinq1911/nonchalant/pkg/auth"
)

// Authenticator decides whether a publish attempt is allowed.
// It is a thin alias around auth.KeySet so RTMP and HTTP playback share one
// implementation. A nil *Authenticator means "anonymous publishing allowed".
type Authenticator = auth.KeySet

// NewAuthenticator builds an Authenticator from a list of pre-shared keys.
// Empty / whitespace-only keys are skipped. If the resulting key set is empty,
// returns nil — callers must treat nil as "auth disabled" so that omitting the
// auth section in YAML preserves the prior anonymous behaviour.
func NewAuthenticator(keys []string) *Authenticator {
	return auth.NewKeySet(keys)
}

// ParseStreamName splits a raw RTMP stream-name string of the form
// "name?key=secret&foo=bar" into the bare name and the value of the "key"
// query parameter. Other query parameters are discarded.
// Returns the original string and "" if no query string is present.
func ParseStreamName(raw string) (name, key string) {
	idx := strings.IndexByte(raw, '?')
	if idx < 0 {
		return raw, ""
	}
	name = raw[:idx]
	q, err := url.ParseQuery(raw[idx+1:])
	if err != nil {
		return name, ""
	}
	return name, q.Get("key")
}

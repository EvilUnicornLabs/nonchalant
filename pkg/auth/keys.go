// If you are AI: This file implements a shared pre-shared-key authenticator
// used by both publisher (RTMP) and subscriber (HTTP/WS/HLS/DASH) auth paths.

package auth

import (
	"crypto/subtle"
	"strings"
)

// KeySet is the set of accepted pre-shared keys.
// A nil *KeySet means "auth disabled" — every key (including the empty string)
// is allowed. Construct with NewKeySet; treat nil as the explicit "off" state
// so that omitting the auth section in YAML preserves anonymous behaviour.
type KeySet struct {
	keys [][]byte
}

// NewKeySet builds a KeySet from a list of pre-shared secrets.
// Empty / whitespace-only entries are skipped. If no keys remain it returns
// nil so the caller can treat absence-of-policy and presence-of-empty-policy
// the same way.
func NewKeySet(keys []string) *KeySet {
	out := &KeySet{keys: make([][]byte, 0, len(keys))}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out.keys = append(out.keys, []byte(k))
	}
	if len(out.keys) == 0 {
		return nil
	}
	return out
}

// Allow reports whether key matches one of the configured secrets.
// A nil receiver always returns true (auth disabled). An empty string never
// matches a configured key. Comparison is constant-time per candidate to
// avoid leaking which key matched via timing.
func (a *KeySet) Allow(key string) bool {
	if a == nil {
		return true
	}
	if key == "" {
		return false
	}
	got := []byte(key)
	for _, want := range a.keys {
		if subtle.ConstantTimeCompare(got, want) == 1 {
			return true
		}
	}
	return false
}

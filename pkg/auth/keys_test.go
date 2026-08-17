// If you are AI: Unit tests for the pre-shared-key set.

package auth

import "testing"

// TestNilKeySetAllowsEverything: nil receiver = auth disabled.
func TestNilKeySetAllowsEverything(t *testing.T) {
	var ks *KeySet
	for _, in := range []string{"", "anything", "longer-key"} {
		if !ks.Allow(in) {
			t.Errorf("nil KeySet should allow %q", in)
		}
	}
}

// TestEmptyAndWhitespaceProduceNil: an empty config means no policy.
func TestEmptyAndWhitespaceProduceNil(t *testing.T) {
	if NewKeySet(nil) != nil {
		t.Error("nil keys should produce nil KeySet")
	}
	if NewKeySet([]string{"  ", ""}) != nil {
		t.Error("blank keys should produce nil KeySet")
	}
}

// TestAllowMatching covers the success / failure paths.
func TestAllowMatching(t *testing.T) {
	ks := NewKeySet([]string{"alpha", "bravo"})
	if ks == nil {
		t.Fatal("expected non-nil")
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"alpha", true},
		{"bravo", true},
		{"charlie", false},
		{"", false},
		{"alphax", false},
	}
	for _, tc := range cases {
		if got := ks.Allow(tc.in); got != tc.want {
			t.Errorf("Allow(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

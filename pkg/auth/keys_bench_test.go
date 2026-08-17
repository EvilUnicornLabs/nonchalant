// If you are AI: Benchmarks for the auth.KeySet hot path. Allow runs on
// every authenticated request — its cost matters at fan-out scale.

package auth

import (
	"fmt"
	"testing"
)

// BenchmarkKeySetAllowMatch measures Allow when the candidate matches the
// LAST configured key. Worst case for the linear scan.
func BenchmarkKeySetAllowMatch(b *testing.B) {
	for _, n := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("keys=%d", n), func(b *testing.B) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = fmt.Sprintf("secret-%04d", i)
			}
			ks := NewKeySet(keys)
			candidate := keys[n-1]
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if !ks.Allow(candidate) {
					b.Fatal("expected match")
				}
			}
		})
	}
}

// BenchmarkKeySetAllowMiss measures Allow when the candidate is wrong.
// All N keys are tested via constant-time compare — same path latency.
func BenchmarkKeySetAllowMiss(b *testing.B) {
	keys := make([]string, 16)
	for i := range keys {
		keys[i] = fmt.Sprintf("secret-%04d", i)
	}
	ks := NewKeySet(keys)
	candidate := "nope-not-a-key-1234"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if ks.Allow(candidate) {
			b.Fatal("unexpected match")
		}
	}
}

// BenchmarkKeySetAllowDisabled is the fast-path: a nil KeySet returns true
// without allocating or scanning. This is the cost of having auth disabled.
func BenchmarkKeySetAllowDisabled(b *testing.B) {
	var ks *KeySet
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !ks.Allow("anything") {
			b.Fatal("nil KeySet should always allow")
		}
	}
}

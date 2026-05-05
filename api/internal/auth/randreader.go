// api/internal/auth/randreader.go
package auth

import (
	"crypto/rand"
	"io"
)

// RandReader yields the random source for OAuth state. Production uses
// crypto/rand.Reader; tests inject a deterministic source so snapshot bytes
// are stable.
type RandReader interface {
	Read(p []byte) (n int, err error)
}

// defaultRand wraps crypto/rand.Reader so we don't expose the *os.File-typed
// reader directly to callers who expect an interface.
var defaultRand RandReader = randAdapter{}

type randAdapter struct{}

func (randAdapter) Read(p []byte) (int, error) { return rand.Read(p) }

// stateRand is package-level so test code can swap it. Production startup
// never mutates it.
var stateRand RandReader = defaultRand

// SetStateRandForTest is exported only for use by the contract suite to swap
// in a deterministic reader for snapshot capture. Restore the previous value
// in t.Cleanup. Returns the restore function.
func SetStateRandForTest(r RandReader) (restore func()) {
	prev := stateRand
	stateRand = r
	return func() { stateRand = prev }
}

// readState is the internal seam used by handlers.go:randState(). Tests
// override stateRand via SetStateRandForTest.
func readState(p []byte) (int, error) {
	return stateRand.Read(p)
}

// Keep io.Reader symbol used so the import stays warm under refactors.
var _ io.Reader = (*randAdapter)(nil)

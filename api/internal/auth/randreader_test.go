package auth

import (
	"testing"
)

type stubRand struct {
	bytes []byte
	read  int
}

func (s *stubRand) Read(p []byte) (int, error) {
	n := copy(p, s.bytes[s.read:])
	s.read += n
	return n, nil
}

func TestSetStateRandForTest_SwapsAndRestores(t *testing.T) {
	original := stateRand
	stub := &stubRand{bytes: make([]byte, 32)} // any 32 bytes
	for i := range stub.bytes {
		stub.bytes[i] = byte(i + 1)
	}

	// Swap and verify stateRand is now the stub.
	restore := SetStateRandForTest(stub)
	if stateRand != stub {
		t.Fatal("SetStateRandForTest did not install the stub")
	}

	// Use readState (the internal seam) to confirm reads come from the stub.
	buf := make([]byte, 4)
	if _, err := readState(buf); err != nil {
		t.Fatalf("readState: %v", err)
	}
	want := []byte{1, 2, 3, 4}
	for i := range want {
		if buf[i] != want[i] {
			t.Fatalf("buf[%d]=%d want %d", i, buf[i], want[i])
		}
	}

	// Restore and verify stateRand is back to the original.
	restore()
	if stateRand != original {
		t.Fatal("restore did not put stateRand back to its original value")
	}
}

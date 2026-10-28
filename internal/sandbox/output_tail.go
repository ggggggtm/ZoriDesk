package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// outputTail drains both output streams while retaining only their last 64 KiB.
// It must report full writes even when discarding older bytes: short writes
// would stop pipe draining and change the command's exit or cancellation behavior.
// The zero value is usable. Each Exec owns one writer; payload storage is fixed
// regardless of total output size. This does not limit the child's own memory.
type outputTail struct {
	mu   sync.Mutex
	data [loopbackOutputLimit]byte
	next int
	size int
}

func (output *outputTail) Write(p []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n := len(p)
	if n >= len(output.data) {
		copy(output.data[:], p[n-len(output.data):])
		output.next, output.size = 0, len(output.data)
		return n, nil
	}
	first := copy(output.data[output.next:], p)
	copy(output.data[:], p[first:])
	output.next = (output.next + n) % len(output.data)
	output.size = min(output.size+n, len(output.data))
	return n, nil
}

// Digest preserves the existing SHA-256-of-suffix contract, not a full-output
// digest. Hash in chronological order without flattening the ring into a copy.
func (output *outputTail) Digest() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	digest := sha256.New()
	if output.size < len(output.data) {
		_, _ = digest.Write(output.data[:output.size])
	} else {
		_, _ = digest.Write(output.data[output.next:])
		_, _ = digest.Write(output.data[:output.next])
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

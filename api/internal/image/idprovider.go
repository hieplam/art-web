// api/internal/image/idprovider.go
package image

import (
	"fmt"

	"github.com/google/uuid"
)

// IDProvider yields per-row IDs for image uploads. Production uses
// uuidIDProvider (a thin wrapper over uuid.NewString); the contract suite
// injects CounterIDProvider so snapshot bytes are stable.
type IDProvider interface {
	NewID() string
}

type uuidIDProvider struct{}

func (uuidIDProvider) NewID() string { return uuid.NewString() }

// NewUUIDProvider returns the production IDProvider.
func NewUUIDProvider() IDProvider { return uuidIDProvider{} }

// CounterIDProvider is a deterministic IDProvider for tests. The Nth call
// returns "00000000-0000-0000-0000-NNNNNNNNNNNN" (12-digit zero-padded N).
type CounterIDProvider struct {
	N int
}

func (c *CounterIDProvider) NewID() string {
	c.N++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", c.N)
}

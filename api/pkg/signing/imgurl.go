// api/pkg/signing/imgurl.go
package signing

import (
	"fmt"
	"strings"
	"time"
)

type URLBuilder struct {
	base string
	key  []byte
	now  func() time.Time
}

func NewURLBuilder(base string, key []byte, now func() time.Time) *URLBuilder {
	return &URLBuilder{base: strings.TrimRight(base, "/"), key: key, now: now}
}

func (b *URLBuilder) Public(storageKey string) string {
	return fmt.Sprintf("%s/img/%s", b.base, storageKey)
}

func (b *URLBuilder) Private(storageKey string, ttl time.Duration) string {
	if !strings.HasPrefix(storageKey, "private/") {
		return ""
	}
	canonical := "/" + storageKey
	exp := b.now().Add(ttl).Unix()
	sig, err := SignCanonical(b.key, canonical, exp)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/img%s?sig=%s&exp=%d", b.base, canonical, sig, exp)
}

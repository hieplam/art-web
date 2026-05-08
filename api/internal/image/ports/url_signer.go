package ports

import "time"

// URLSigner produces externally-visible image URLs. Public URLs are unsigned;
// Private URLs include a TTL-bounded HMAC signature. Implemented by
// pkg/signing.URLBuilder.
type URLSigner interface {
	Public(storageKey string) string
	Private(storageKey string, ttl time.Duration) string
}

// api/internal/auth/sign.go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// SignCanonical produces the lowercase-hex HMAC-SHA256 of "v1|<canonicalPath>|<exp>".
func SignCanonical(key []byte, canonicalPath string, exp int64) (string, error) {
	if err := ValidateCanonicalPath(canonicalPath); err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, key)
	h.Write([]byte("v1|"))
	h.Write([]byte(canonicalPath))
	h.Write([]byte{'|'})
	h.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ValidateCanonicalPath(p string) error {
	if p == "" || p[0] != '/' {
		return fmt.Errorf("must start with /")
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("must not end with /")
	}
	if strings.Contains(p, "//") || strings.Contains(p, "/../") || strings.Contains(p, "/./") {
		return fmt.Errorf("contains forbidden segment")
	}
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '/' || r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("char %q not allowed", r)
		}
	}
	return nil
}

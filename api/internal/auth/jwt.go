// api/internal/auth/jwt.go
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWT struct {
	key []byte
	now func() time.Time
}

func NewJWT(key []byte, now func() time.Time) *JWT {
	if now == nil {
		now = time.Now
	}
	return &JWT{key: key, now: now}
}

func (j *JWT) Issue(sub string, ttl time.Duration) (string, error) {
	now := j.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})
	return tok.SignedString(j.key)
}

func (j *JWT) Verify(token string) (string, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected alg")
			}
			return j.key, nil
		},
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return "", err
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	return claims.Subject, nil
}

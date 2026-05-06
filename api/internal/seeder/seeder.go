// Package seeder orchestrates the deterministic dev/test fixture creation
// that used to live behind POST /dev/seed. The HTTP route is gone (Task 10
// of the Phase 1 refactor); callers now invoke Run directly:
//
//   - cmd/seeder is a thin CLI wrapper (binary).
//   - The contract suite imports this package and calls Run with the
//     *gorm.DB owned by infratest.BootApp.
//
// Behavior is intentionally identical to the legacy DevSeed handler so the
// 56-cell byte-strict contract goldens keep passing.
package seeder

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	authservice "local/art-web/api/internal/auth/service"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

// Output mirrors the legacy /dev/seed JSON response so existing tooling that
// reads aliceCookie / bobCookie / aliceSlug / bobSlug / pId / qId continues
// to work after the binary swap. Field tags use camelCase to match.
type Output struct {
	AliceCookie string `json:"aliceCookie"`
	BobCookie   string `json:"bobCookie"`
	AliceSlug   string `json:"aliceSlug"`
	BobSlug     string `json:"bobSlug"`
	PID         string `json:"pId"`
	QID         string `json:"qId"`
}

// Run seeds two users (alice + bob), two artworks (public P + private Q,
// owned by alice), one image per artwork, and (optionally) `many` extra
// public artworks for bulk feed tests. It returns the cookie/slug/id values
// callers need to drive subsequent requests.
//
// suffix controls the alice-/bob- slug suffix for determinism. Pass "" to
// get a random hex; tests should always pass a fixed value.
//
// many caps at 200 to keep contract-suite seed time bounded; values above
// the cap silently clamp.
func Run(
	ctx context.Context,
	db *gorm.DB,
	store infrastorage.Storage,
	jwt *authservice.JWT,
	suffix string,
	many int,
) (*Output, error) {
	users := userpostgres.NewRepo(db)
	arts := artworkpostgres.NewRepo(db)
	tags := artworkpostgres.NewTagsRepo(db)
	images := imagepostgres.NewRepo(db)

	if suffix == "" {
		suffix = randHex(4)
	}
	aliceSlug := "alice-" + suffix
	bobSlug := "bob-" + suffix

	aliceID, err := users.UpsertOAuth(ctx, "test", "alice-"+suffix, "alice@test", aliceSlug, "")
	if err != nil {
		return nil, err
	}
	bobID, err := users.UpsertOAuth(ctx, "test", "bob-"+suffix, "bob@test", bobSlug, "")
	if err != nil {
		return nil, err
	}

	pID, err := arts.Create(ctx, aliceID, "Public P", "Seeded public artwork P", "public")
	if err != nil {
		return nil, err
	}
	qID, err := arts.Create(ctx, aliceID, "Private Q", "Seeded private artwork Q", "private")
	if err != nil {
		return nil, err
	}

	if err := tags.SetTags(ctx, pID, []string{"t"}); err != nil {
		return nil, err
	}
	if err := tags.SetTags(ctx, qID, []string{"t"}); err != nil {
		return nil, err
	}

	if err := attachSeedImage(ctx, store, images, pID, "public", "seed-p", 0); err != nil {
		return nil, err
	}
	if err := attachSeedImage(ctx, store, images, qID, "private", "seed-q", 0); err != nil {
		return nil, err
	}

	if many > 0 {
		const maxMany = 200
		if many > maxMany {
			many = maxMany
		}
		for i := 0; i < many; i++ {
			id, err := arts.Create(ctx, aliceID, fmt.Sprintf("Bulk %d", i), "Seeded bulk artwork", "public")
			if err != nil {
				return nil, err
			}
			if err := attachSeedImage(ctx, store, images, id, "public", fmt.Sprintf("seed-bulk-%d", i), 0); err != nil {
				return nil, err
			}
		}
	}

	aliceJWT, err := jwt.Issue(aliceID, time.Hour)
	if err != nil {
		return nil, err
	}
	bobJWT, err := jwt.Issue(bobID, time.Hour)
	if err != nil {
		return nil, err
	}

	return &Output{
		AliceCookie: "auth=" + aliceJWT,
		BobCookie:   "auth=" + bobJWT,
		AliceSlug:   aliceSlug,
		BobSlug:     bobSlug,
		PID:         pID,
		QID:         qID,
	}, nil
}

// ParseMany parses the legacy ?many= query value with the same lenient rules
// the HTTP handler used: empty / non-numeric / negative all collapse to 0.
// Exposed so cmd/seeder can mirror the old default-on-empty semantics.
func ParseMany(s string) int {
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}

func attachSeedImage(
	ctx context.Context,
	store infrastorage.Storage,
	images *imagepostgres.Repo,
	artworkID, visibility, clientID string,
	position int,
) error {
	imageID := uuid.NewString()
	src, err := pickSeedSource(clientID)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s/%s/%s%s", visibility, artworkID, imageID, src.ext)

	if err := store.Put(ctx, key, bytes.NewReader(src.bytes), src.contentType); err != nil {
		return err
	}
	_, err = images.Insert(ctx, imagepostgres.InsertInput{
		ID:            imageID,
		ArtworkID:     artworkID,
		ClientImageID: clientID,
		ContentType:   src.contentType,
		StorageKey:    key,
		SourceSHA256:  src.sha256Hex,
		Position:      position,
		Width:         src.width,
		Height:        src.height,
		ByteSize:      len(src.bytes),
		Blurhash:      src.blurhash,
	})
	return err
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// seedFromString hashes the input string into a stable uint64 RNG seed.
// Deterministic across runs and platforms.
func seedFromString(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	return binary.LittleEndian.Uint64(sum[:8])
}

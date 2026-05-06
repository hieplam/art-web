package postgres

import (
	"testing"
	"time"
)

func TestToDomainArtwork_FieldsCopyWithCreatedAtPointer(t *testing.T) {
	desc := "alpha"
	pub := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cre := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &artworkModel{
		ID:            "art1",
		UserID:        "u1",
		Title:         "title",
		Description:   &desc,
		Visibility:    "public",
		CoverPosition: 3,
		CreatedAt:     cre,
		PublishedAt:   &pub,
	}
	a := toDomainArtwork(m)
	if a.ID != "art1" || a.UserID != "u1" || a.Title != "title" {
		t.Fatalf("ID/UserID/Title mismatch: %+v", a)
	}
	if a.Description == nil || *a.Description != "alpha" {
		t.Fatalf("description mismatch: %+v", a.Description)
	}
	if a.Visibility != "public" || a.CoverPosition != 3 {
		t.Fatalf("vis/cover mismatch: %+v", a)
	}
	if a.CreatedAt == nil || !a.CreatedAt.Equal(cre) {
		t.Fatalf("created_at mismatch: %+v", a.CreatedAt)
	}
	if a.PublishedAt == nil || !a.PublishedAt.Equal(pub) {
		t.Fatalf("published_at mismatch: %+v", a.PublishedAt)
	}
}

func TestToDomainArtwork_NilModel_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil model")
		}
	}()
	toDomainArtwork(nil)
}

func TestArtworkTagModel_TableName(t *testing.T) {
	if (artworkTagModel{}).TableName() != "artwork_tags" {
		t.Fatal("artwork_tags table name drift")
	}
	if (artworkModel{}).TableName() != "artworks" {
		t.Fatal("artworks table name drift")
	}
	if (artworkImageModel{}).TableName() != "artwork_images" {
		t.Fatal("artwork_images table name drift")
	}
	if (tagModel{}).TableName() != "tags" {
		t.Fatal("tags table name drift")
	}
}

func TestFlipPrefixError_Message(t *testing.T) {
	e := &flipPrefixError{src: "private/x/y.jpg", fromVis: "public"}
	want := "storage_key private/x/y.jpg does not match artwork visibility public"
	if e.Error() != want {
		t.Fatalf("got %q want %q", e.Error(), want)
	}
}

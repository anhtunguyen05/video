package domain

import (
	"testing"
	"time"
)

func TestNewVideoValidatesTitle(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	if _, err := NewVideo("video-id", "owner-id", "   ", nil, now); err == nil {
		t.Fatal("expected empty title to be rejected")
	}

	video, err := NewVideo("video-id", "owner-id", "  Summer Trip  ", nil, now)
	if err != nil {
		t.Fatalf("NewVideo() error = %v", err)
	}
	if video.Title != "Summer Trip" {
		t.Fatalf("Title = %q, want trimmed title", video.Title)
	}
	if video.Status != StatusCreated || video.ProcessingVersion != 1 {
		t.Fatalf("unexpected initial state: %#v", video)
	}
}

func TestVideoDeleteTransition(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	video, err := NewVideo("video-id", "owner-id", "Summer Trip", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	deletedAt := now.Add(time.Minute)
	if err := video.Delete(deletedAt); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if video.Status != StatusDeleted || video.DeletedAt == nil || !video.DeletedAt.Equal(deletedAt) {
		t.Fatalf("unexpected deleted state: %#v", video)
	}
	if err := video.Delete(deletedAt.Add(time.Minute)); err != ErrInvalidState {
		t.Fatalf("second Delete() error = %v, want ErrInvalidState", err)
	}
}

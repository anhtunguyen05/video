package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestPlanRenditionsUsesShortEdgeForLandscape(t *testing.T) {
	planned, err := PlanRenditions(1280, 720)
	if err != nil {
		t.Fatalf("PlanRenditions() error = %v", err)
	}
	want := []Rendition{
		{Name: "360p", Width: 640, Height: 360, Codec: PlannedRenditionCodec},
		{Name: "720p", Width: 1280, Height: 720, Codec: PlannedRenditionCodec},
	}
	if !reflect.DeepEqual(planned, want) {
		t.Fatalf("PlanRenditions() = %#v, want %#v", planned, want)
	}
}

func TestPlanRenditionsPreservesPortraitOrientation(t *testing.T) {
	planned, err := PlanRenditions(1080, 1920)
	if err != nil {
		t.Fatalf("PlanRenditions() error = %v", err)
	}
	want := []Rendition{
		{Name: "360p", Width: 360, Height: 640, Codec: PlannedRenditionCodec},
		{Name: "720p", Width: 720, Height: 1280, Codec: PlannedRenditionCodec},
		{Name: "1080p", Width: 1080, Height: 1920, Codec: PlannedRenditionCodec},
	}
	if !reflect.DeepEqual(planned, want) {
		t.Fatalf("PlanRenditions() = %#v, want %#v", planned, want)
	}
}

func TestPlanRenditionsDoesNotUpscale(t *testing.T) {
	planned, err := PlanRenditions(320, 568)
	if err != nil {
		t.Fatalf("PlanRenditions() error = %v", err)
	}
	if len(planned) != 0 {
		t.Fatalf("PlanRenditions() = %#v, want no renditions", planned)
	}
}

func TestPlanRenditionsRoundsTheLongEdgeDownToEven(t *testing.T) {
	planned, err := PlanRenditions(853, 480)
	if err != nil {
		t.Fatalf("PlanRenditions() error = %v", err)
	}
	want := []Rendition{{Name: "360p", Width: 638, Height: 360, Codec: PlannedRenditionCodec}}
	if !reflect.DeepEqual(planned, want) {
		t.Fatalf("PlanRenditions() = %#v, want %#v", planned, want)
	}
}

func TestPlanRenditionsRejectsInvalidSourceDimensions(t *testing.T) {
	_, err := PlanRenditions(0, 720)
	if !errors.Is(err, ErrInvalidSourceDimensions) {
		t.Fatalf("PlanRenditions() error = %v, want %v", err, ErrInvalidSourceDimensions)
	}
}

package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidSourceDimensions = errors.New("invalid source dimensions")

const PlannedRenditionCodec = "h264"

var renditionTargets = []int{360, 720, 1080}

type Rendition struct {
	Name   string
	Width  int
	Height int
	Codec  string
}

// PlanRenditions creates an ordered ladder whose target is the source's shorter
// edge. This keeps portrait videos portrait while preserving the conventional
// 360p/720p/1080p behaviour for landscape sources.
func PlanRenditions(sourceWidth, sourceHeight int) ([]Rendition, error) {
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return nil, ErrInvalidSourceDimensions
	}

	shortEdge, longEdge := sourceWidth, sourceHeight
	portraitOrSquare := sourceWidth <= sourceHeight
	if !portraitOrSquare {
		shortEdge, longEdge = sourceHeight, sourceWidth
	}

	planned := make([]Rendition, 0, len(renditionTargets))
	for _, target := range renditionTargets {
		if target > shortEdge {
			continue
		}
		longOutputEdge := evenFloor(int64(longEdge) * int64(target) / int64(shortEdge))
		rendition := Rendition{
			Name:  renditionName(target),
			Codec: PlannedRenditionCodec,
		}
		if portraitOrSquare {
			rendition.Width, rendition.Height = target, longOutputEdge
		} else {
			rendition.Width, rendition.Height = longOutputEdge, target
		}
		planned = append(planned, rendition)
	}
	return planned, nil
}

func renditionName(target int) string {
	return fmt.Sprintf("%dp", target)
}

func evenFloor(value int64) int {
	return int(value - value%2)
}

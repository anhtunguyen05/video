package ports

import "context"

type VideoUploaded struct {
	VideoID           string `json:"video_id"`
	OwnerID           string `json:"owner_id"`
	SourceObjectKey   string `json:"source_object_key"`
	ProcessingVersion int    `json:"processing_version"`
	CorrelationID     string `json:"-"`
	CausationID       string `json:"-"`
}

type Publisher interface {
	PublishVideoUploaded(context.Context, VideoUploaded) error
}

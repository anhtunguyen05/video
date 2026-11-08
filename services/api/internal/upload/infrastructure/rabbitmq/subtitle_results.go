package rabbitmq

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"video/services/api/internal/platform/ids"
)

type subtitleResult struct {
	MessageID     string `json:"message_id"`
	MessageType   string `json:"message_type"`
	SchemaVersion int    `json:"schema_version"`
	Payload       struct {
		JobID           string  `json:"job_id"`
		VideoID         string  `json:"video_id"`
		OwnerID         string  `json:"owner_id"`
		SourceVersion   int     `json:"source_version"`
		Language        string  `json:"language"`
		Status          string  `json:"status"`
		OutputObjectKey *string `json:"output_object_key"`
	} `json:"payload"`
}

func parseSubtitleResult(body []byte, brokerID string) (subtitleResult, error) {
	var event subtitleResult
	if len(body) > 16384 || json.Unmarshal(body, &event) != nil {
		return event, fmt.Errorf("invalid result envelope")
	}
	p := event.Payload
	if event.MessageType != "subtitle.completed.v1" || event.SchemaVersion != 1 || !ids.IsUUID(event.MessageID) || brokerID != event.MessageID ||
		!ids.IsUUID(p.JobID) || !ids.IsUUID(p.VideoID) || !ids.IsUUID(p.OwnerID) || p.SourceVersion <= 0 || p.Language != "vi" {
		return event, fmt.Errorf("invalid result identity")
	}
	if p.Status == "READY" {
		if p.OutputObjectKey == nil {
			return event, fmt.Errorf("missing output")
		}
		prefix := fmt.Sprintf("users/%s/videos/%s/subtitles/v%d/runs/", p.OwnerID, p.VideoID, p.SourceVersion)
		if !strings.HasPrefix(*p.OutputObjectKey, prefix) {
			return event, fmt.Errorf("invalid output location")
		}
		parts := strings.Split(strings.TrimPrefix(*p.OutputObjectKey, prefix), "/")
		if len(parts) != 2 || !ids.IsUUID(parts[0]) || parts[1] != "vi.vtt" {
			return event, fmt.Errorf("invalid output identity")
		}
	} else if (p.Status != "NO_AUDIO" && p.Status != "NO_SPEECH" && p.Status != "FAILED") || p.OutputObjectKey != nil {
		return event, fmt.Errorf("invalid result state")
	}
	return event, nil
}

func applySubtitleResult(ctx context.Context, db *sql.DB, delivery amqp091.Delivery) error {
	event, err := parseSubtitleResult(delivery.Body, delivery.MessageId)
	if err != nil {
		digest := sha256.Sum256(delivery.Body)
		_, err = db.ExecContext(ctx, `INSERT INTO subtitle_result_quarantine(digest,payload) VALUES ($1,$2) ON CONFLICT DO NOTHING`, hex.EncodeToString(digest[:]), delivery.Body)
		return err
	}
	p := event.Payload
	_, err = db.ExecContext(ctx, `UPDATE subtitle_requests s SET status=$1, output_object_key=$2, updated_at=now()
		WHERE s.id=$3 AND s.video_id=$4 AND s.owner_id=$5 AND s.source_version=$6 AND s.language='vi'
		AND s.status IN ('PENDING','PROCESSING') AND EXISTS
		(SELECT 1 FROM videos v WHERE v.id=s.video_id AND v.owner_id=s.owner_id AND v.deleted_at IS NULL AND v.processing_version=s.source_version)`,
		p.Status, p.OutputObjectKey, p.JobID, p.VideoID, p.OwnerID, p.SourceVersion)
	return err
}

func RunSubtitleResultLoop(ctx context.Context, db *sql.DB, url string, logger *slog.Logger) {
	for ctx.Err() == nil {
		if err := consumeSubtitleResults(ctx, db, url); err != nil && ctx.Err() == nil {
			logger.Error("subtitle result consumer restarting", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func consumeSubtitleResults(ctx context.Context, db *sql.DB, url string) error {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err = ch.ExchangeDeclare("subtitle.events", "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err = ch.QueueDeclare("media.subtitle-completed.v1", true, false, false, false, nil); err != nil {
		return err
	}
	if err = ch.QueueBind("media.subtitle-completed.v1", "subtitle.completed.v1", "subtitle.events", false, nil); err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.ConsumeWithContext(ctx, "media.subtitle-completed.v1", "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for delivery := range deliveries {
		if err := applySubtitleResult(ctx, db, delivery); err != nil {
			return err
		}
		if err := delivery.Ack(false); err != nil {
			return err
		}
	}
	return ctx.Err()
}

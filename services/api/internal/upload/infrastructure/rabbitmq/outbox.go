package rabbitmq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type outboxEvent struct {
	id, exchange, routing string
	body                  []byte
}

func RunOutboxLoop(ctx context.Context, db *sql.DB, url string, logger *slog.Logger) {
	for ctx.Err() == nil {
		publisher, err := NewPublisher(url)
		if err == nil {
			err = publisher.RunOutbox(ctx, db)
			_ = publisher.Close()
		}
		if err != nil && ctx.Err() == nil {
			logger.Error("upload outbox reconnecting", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// RunOutbox publishes committed upload events. A confirm can be followed by a
// crash before published_at is stored, so consumers must still deduplicate.
func (publisher *Publisher) RunOutbox(ctx context.Context, db *sql.DB) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := publisher.publishNext(ctx, db); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (publisher *Publisher) publishNext(ctx context.Context, db *sql.DB) error {
	var event outboxEvent
	err := db.QueryRowContext(ctx, `
		UPDATE outbox_events SET locked_until = now() + interval '30 seconds'
		WHERE id = (
			SELECT id FROM outbox_events
			WHERE exchange_name = 'video.events' AND published_at IS NULL
			  AND next_attempt_at <= now()
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING id::text, exchange_name, routing_key, payload::text
	`).Scan(&event.id, &event.exchange, &event.routing, &event.body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim upload outbox: %w", err)
	}
	if err := publisher.publishConfirmed(ctx, event); err != nil {
		_, updateErr := db.ExecContext(ctx, `
			UPDATE outbox_events SET attempt_count = attempt_count + 1,
				next_attempt_at = now() + make_interval(secs => LEAST(300, power(2, LEAST(attempt_count + 1, 8))::integer)),
				locked_until = NULL WHERE id = $1
		`, event.id)
		if updateErr != nil {
			return fmt.Errorf("publish upload event: %v; record retry: %w", err, updateErr)
		}
		if publisher.channel.IsClosed() || publisher.connection.IsClosed() {
			return err
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `UPDATE outbox_events SET published_at = now(), locked_until = NULL WHERE id = $1`, event.id)
	return err
}

func (publisher *Publisher) publishConfirmed(ctx context.Context, event outboxEvent) error {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	if err := publisher.channel.PublishWithContext(ctx, event.exchange, event.routing, true, false, amqp091.Publishing{
		ContentType: "application/json", DeliveryMode: amqp091.Persistent,
		MessageId: event.id, Body: event.body,
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-publisher.returns:
		return errors.New("RabbitMQ returned unroutable upload event")
	case confirm, ok := <-publisher.confirms:
		if !ok || !confirm.Ack {
			return errors.New("RabbitMQ did not confirm upload event")
		}
		select {
		case <-publisher.returns:
			return errors.New("RabbitMQ returned unroutable upload event")
		default:
		}
		return nil
	}
}

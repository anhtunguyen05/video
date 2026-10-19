package rabbitmq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	DeadExchange   = "processing.dead"
	DeadQueue      = "processing.dead.v1"
	DeadRoutingKey = "processing.dead.v1"
)

type DeadPublisher struct {
	connection *amqp091.Connection
	channel    *amqp091.Channel
	confirms   <-chan amqp091.Confirmation
	returns    <-chan amqp091.Return
}

func NewDeadPublisher(url string) (*DeadPublisher, error) {
	connection, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	closeOnError := func(err error) (*DeadPublisher, error) {
		_ = channel.Close()
		_ = connection.Close()
		return nil, err
	}
	if err := channel.ExchangeDeclare(DeadExchange, amqp091.ExchangeDirect, true, false, false, false, nil); err != nil {
		return closeOnError(err)
	}
	if _, err := channel.QueueDeclare(DeadQueue, true, false, false, false, nil); err != nil {
		return closeOnError(err)
	}
	if err := channel.QueueBind(DeadQueue, DeadRoutingKey, DeadExchange, false, nil); err != nil {
		return closeOnError(err)
	}
	if err := channel.Confirm(false); err != nil {
		return closeOnError(err)
	}
	return &DeadPublisher{connection: connection, channel: channel,
		confirms: channel.NotifyPublish(make(chan amqp091.Confirmation, 1)),
		returns:  channel.NotifyReturn(make(chan amqp091.Return, 1))}, nil
}

func (publisher *DeadPublisher) Close() error {
	if publisher == nil {
		return nil
	}
	_ = publisher.channel.Close()
	return publisher.connection.Close()
}

func (publisher *DeadPublisher) Run(ctx context.Context, db *sql.DB) error {
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

func (publisher *DeadPublisher) publishNext(ctx context.Context, db *sql.DB) error {
	var id, exchange, routing string
	var body []byte
	err := db.QueryRowContext(ctx, `
		UPDATE outbox_events SET locked_until = now() + interval '30 seconds'
		WHERE id = (
			SELECT id FROM outbox_events WHERE exchange_name = 'processing.dead'
			  AND published_at IS NULL AND next_attempt_at <= now()
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING id::text, exchange_name, routing_key, payload::text
	`).Scan(&id, &exchange, &routing, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	err = publisher.channel.PublishWithContext(ctx, exchange, routing, true, false, amqp091.Publishing{
		ContentType: "application/json", DeliveryMode: amqp091.Persistent, MessageId: id, Body: body,
	})
	if err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-publisher.returns:
			err = errors.New("unroutable dead letter")
		case confirm, ok := <-publisher.confirms:
			if !ok || !confirm.Ack {
				err = errors.New("dead letter was not confirmed")
			}
			if err == nil {
				select {
				case <-publisher.returns:
					err = errors.New("unroutable dead letter")
				default:
				}
			}
		}
	}
	if err != nil {
		_, updateErr := db.ExecContext(ctx, `UPDATE outbox_events SET attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => LEAST(300, power(2, LEAST(attempt_count + 1, 8))::integer)),
			locked_until = NULL WHERE id = $1`, id)
		if updateErr != nil {
			return fmt.Errorf("publish dead letter: %v; record retry: %w", err, updateErr)
		}
		if publisher.channel.IsClosed() || publisher.connection.IsClosed() {
			return err
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `UPDATE outbox_events SET published_at = now(), locked_until = NULL WHERE id = $1`, id)
	return err
}

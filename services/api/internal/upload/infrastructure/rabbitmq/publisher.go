package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"video/services/api/internal/platform/ids"
	"video/services/api/internal/upload/ports"
)

const (
	ExchangeName = "video.events"
	QueueName    = "processing.video-uploaded.v1"
	RoutingKey   = "video.uploaded.v1"
)

type Publisher struct {
	connection *amqp091.Connection
	channel    *amqp091.Channel
	mutex      sync.Mutex
}

type envelope struct {
	MessageID     string              `json:"message_id"`
	MessageType   string              `json:"message_type"`
	SchemaVersion int                 `json:"schema_version"`
	OccurredAt    time.Time           `json:"occurred_at"`
	CorrelationID string              `json:"correlation_id"`
	CausationID   string              `json:"causation_id"`
	Payload       ports.VideoUploaded `json:"payload"`
}

func NewPublisher(url string) (*Publisher, error) {
	connection, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.ExchangeDeclare(ExchangeName, amqp091.ExchangeTopic, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(QueueName, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ queue: %w", err)
	}
	if err := channel.QueueBind(QueueName, RoutingKey, ExchangeName, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("bind RabbitMQ queue: %w", err)
	}
	return &Publisher{connection: connection, channel: channel}, nil
}

func (publisher *Publisher) PublishVideoUploaded(ctx context.Context, event ports.VideoUploaded) error {
	messageID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("create message id: %w", err)
	}
	body, err := json.Marshal(envelope{
		MessageID:     messageID,
		MessageType:   RoutingKey,
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: event.CorrelationID,
		CausationID:   event.CausationID,
		Payload:       event,
	})
	if err != nil {
		return fmt.Errorf("encode video uploaded event: %w", err)
	}

	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	if err := publisher.channel.PublishWithContext(ctx, ExchangeName, RoutingKey, false, false, amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		MessageId:    messageID,
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish video uploaded event: %w", err)
	}
	return nil
}

func (publisher *Publisher) Close() error {
	if publisher == nil {
		return nil
	}
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	channelErr := publisher.channel.Close()
	connectionErr := publisher.connection.Close()
	if channelErr != nil {
		return channelErr
	}
	return connectionErr
}

var _ ports.Publisher = (*Publisher)(nil)

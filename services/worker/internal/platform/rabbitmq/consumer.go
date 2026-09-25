package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbitmq/amqp091-go"
	processingapplication "video/services/worker/internal/processing/application"
)

const (
	ExchangeName = "video.events"
	QueueName    = "processing.video-uploaded.v1"
	RoutingKey   = "video.uploaded.v1"
)

type Consumer struct {
	connection *amqp091.Connection
	channel    *amqp091.Channel
}

func NewConsumer(url string) (*Consumer, error) {
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
	if err := channel.Qos(1, 0, false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("configure RabbitMQ prefetch: %w", err)
	}
	return &Consumer{connection: connection, channel: channel}, nil
}

func (consumer *Consumer) Run(ctx context.Context, service *processingapplication.Service) error {
	deliveries, err := consumer.channel.ConsumeWithContext(ctx, QueueName, "", false, false, false, false, nil)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return fmt.Errorf("consume RabbitMQ messages: %w", err)
	}
	for delivery := range deliveries {
		err := service.HandleVideoUploaded(ctx, delivery.Body)
		if err == nil {
			if ackErr := delivery.Ack(false); ackErr != nil {
				return fmt.Errorf("ack RabbitMQ message: %w", ackErr)
			}
			continue
		}
		if errors.Is(err, processingapplication.ErrInvalidMessage) {
			if nackErr := delivery.Nack(false, false); nackErr != nil {
				return fmt.Errorf("reject invalid RabbitMQ message: %w", nackErr)
			}
			continue
		}
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			return fmt.Errorf("requeue RabbitMQ message: %w", nackErr)
		}
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (consumer *Consumer) Close() error {
	if consumer == nil {
		return nil
	}
	channelErr := consumer.channel.Close()
	connectionErr := consumer.connection.Close()
	if channelErr != nil {
		return channelErr
	}
	return connectionErr
}

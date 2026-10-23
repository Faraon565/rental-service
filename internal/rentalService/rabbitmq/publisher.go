package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	rentalService "rental-service/internal/rentalService"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	queueName string
}

func NewPublisher(rabbitURL, queueName string) (*Publisher, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open RabbitMQ channel: %w", err)
	}

	_, err = channel.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to declare RabbitMQ queue: %w", err)
	}

	return &Publisher{conn: conn, channel: channel, queueName: queueName}, nil
}

func (p *Publisher) PublishBookingCreated(ctx context.Context, bookingID int64) error {
	event := rentalService.BookingCreatedEvent{BookingID: bookingID}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal booking event: %w", err)
	}

	return p.channel.PublishWithContext(
		ctx,
		"",
		p.queueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (p *Publisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}

	if p.conn != nil {
		return p.conn.Close()
	}

	return nil
}

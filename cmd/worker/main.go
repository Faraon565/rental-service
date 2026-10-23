package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	rentalService "rental-service/internal/rentalService"
	"rental-service/internal/rentalService/repository"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

const bookingQueueName = "booking_created"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://rental:rental@localhost:5432/rental_db1?sslmode=disable"
	}

	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	if err := db.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	logger.Info("worker connected to PostgreSQL")

	repo := repository.New(db)
	defer repo.Close()

	if err := repo.Init(ctx); err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		logger.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	logger.Info("worker connected to RabbitMQ")

	channel, err := conn.Channel()
	if err != nil {
		logger.Error("failed to open RabbitMQ channel", "error", err)
		os.Exit(1)
	}
	defer channel.Close()

	_, err = channel.QueueDeclare(
		bookingQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.Error("failed to declare queue", "error", err)
		os.Exit(1)
	}

	messages, err := channel.Consume(
		bookingQueueName,
		"booking-worker",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.Error("failed to start consumer", "error", err)
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("booking worker started", "queue", bookingQueueName)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				logger.Info("RabbitMQ messages channel closed")
				return
			}

			if err := processMessage(ctx, repo, message); err != nil {
				logger.Error("failed to process booking message", "error", err)
				_ = message.Nack(false, true)
				continue
			}

			_ = message.Ack(false)

		case <-stop:
			logger.Info("booking worker stopped")
			return
		}
	}
}

func processMessage(ctx context.Context, repo rentalService.Repository, message amqp.Delivery) error {
	var event rentalService.BookingCreatedEvent

	if err := json.Unmarshal(message.Body, &event); err != nil {
		return err
	}

	if event.BookingID <= 0 {
		return nil
	}

	slog.Info("booking message received", "booking_id", event.BookingID)

	if err := repo.UpdateBookingStatus(ctx, event.BookingID, "processing"); err != nil {
		return err
	}

	time.Sleep(3 * time.Second)

	if err := repo.UpdateBookingStatus(ctx, event.BookingID, "approved"); err != nil {
		return err
	}

	slog.Info("booking approved", "booking_id", event.BookingID)
	return nil
}

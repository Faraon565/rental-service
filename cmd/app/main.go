package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	httpDelivery "rental-service/internal/rentalService/delivery/http"
	"rental-service/internal/rentalService/rabbitmq"
	"rental-service/internal/rentalService/repository"
	"rental-service/internal/rentalService/usecase"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	logger.Info("connected to PostgreSQL")

	repo := repository.New(db)
	defer repo.Close()

	if err := repo.Init(ctx); err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	logger.Info("database initialized")

	publisher, err := rabbitmq.NewPublisher(rabbitURL, bookingQueueName)
	if err != nil {
		logger.Error("failed to initialize RabbitMQ publisher", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			logger.Error("failed to close RabbitMQ publisher", "error", err)
		}
	}()
	logger.Info("connected to RabbitMQ", "queue", bookingQueueName)

	uc := usecase.New(repo, publisher)
	h := httpDelivery.New(uc)

	mux := http.NewServeMux()
	mux.HandleFunc("/test", h.Test())
	mux.HandleFunc("/dbtest", h.DBTest())
	mux.HandleFunc("/auth/register", h.Register())
	mux.HandleFunc("/auth/login", h.Login())
	mux.HandleFunc("/bookings/create", h.JWTMiddleware(h.CreateBooking()))
	mux.HandleFunc("/bookings/list", h.JWTMiddleware(h.GetBookings()))

	mux.HandleFunc("/ui", h.UI())
	mux.HandleFunc("/ui/register", h.UIRegister())
	mux.HandleFunc("/ui/login", h.UILogin())
	mux.HandleFunc("/ui/logout", h.UILogout())
	mux.HandleFunc("/ui/properties/create", h.UICreateProperty())
	mux.HandleFunc("/ui/bookings/create", h.UICreateBooking())
	mux.Handle("/ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.Dir("web/static"))))

	mux.Handle("/metrics", httpDelivery.MetricsHandler())

	server := &http.Server{
		Addr:    ":8080",
		Handler: httpDelivery.MetricsMiddleware(mux),
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server started", "addr", server.Addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit

	logger.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}

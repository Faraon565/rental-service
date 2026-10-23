package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	AppBaseURL     string
	DatabaseURL    string
	UsersCount     int
	Iterations     int
	Delay          time.Duration
	RequestTimeout time.Duration
}

type testUser struct {
	ID         int64
	Email      string
	Password   string
	Token      string
	PropertyID int64
}

type apiClient struct {
	baseURL string
	client  *http.Client
}

type loginResponse struct {
	Token string `json:"token"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := waitPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("loadgen failed to connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	client := &apiClient{
		baseURL: cfg.AppBaseURL,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}

	if err := waitApp(ctx, client); err != nil {
		logger.Error("loadgen failed to connect to app", "error", err)
		os.Exit(1)
	}

	users, err := prepareUsers(ctx, db, client, cfg.UsersCount)
	if err != nil {
		logger.Error("loadgen failed to prepare users", "error", err)
		os.Exit(1)
	}

	logger.Info(
		"loadgen started",
		"users", len(users),
		"iterations", cfg.Iterations,
		"delay", cfg.Delay.String(),
		"app_base_url", cfg.AppBaseURL,
	)

	var wg sync.WaitGroup

	for _, user := range users {
		wg.Add(1)

		go func(user testUser) {
			defer wg.Done()
			runUserLoad(ctx, client, user, cfg)
		}(user)
	}

	wg.Wait()
	logger.Info("loadgen stopped")
}

func loadConfig() config {
	return config{
		AppBaseURL:     getEnv("APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://rental:rental@localhost:5432/rental_db1?sslmode=disable"),
		UsersCount:     getEnvInt("LOAD_USERS", 5),
		Iterations:     getEnvInt("LOAD_ITERATIONS", 0),
		Delay:          time.Duration(getEnvInt("LOAD_DELAY_MS", 1000)) * time.Millisecond,
		RequestTimeout: time.Duration(getEnvInt("LOAD_TIMEOUT_SECONDS", 5)) * time.Second,
	}
}

func prepareUsers(ctx context.Context, db *pgxpool.Pool, client *apiClient, count int) ([]testUser, error) {
	runID := time.Now().UnixNano()
	users := make([]testUser, 0, count)

	for i := 1; i <= count; i++ {
		user := testUser{
			Email:    fmt.Sprintf("loadgen_%d_%d@test.local", runID, i),
			Password: "123456",
		}

		if err := client.register(ctx, user.Email, user.Password, fmt.Sprintf("Load User %d", i)); err != nil {
			return nil, err
		}

		token, err := client.login(ctx, user.Email, user.Password)
		if err != nil {
			return nil, err
		}
		user.Token = token

		userID, err := getUserID(ctx, db, user.Email)
		if err != nil {
			return nil, err
		}
		user.ID = userID

		propertyID, err := createProperty(ctx, db, user.ID, i)
		if err != nil {
			return nil, err
		}
		user.PropertyID = propertyID

		users = append(users, user)

		slog.Info(
			"loadgen user prepared",
			"email", user.Email,
			"user_id", user.ID,
			"property_id", user.PropertyID,
		)
	}

	return users, nil
}

func runUserLoad(ctx context.Context, client *apiClient, user testUser, cfg config) {
	iteration := 0

	for {
		if ctx.Err() != nil {
			return
		}

		if cfg.Iterations > 0 && iteration >= cfg.Iterations {
			return
		}

		iteration++

		if err := client.dbTest(ctx, fmt.Sprintf("loadgen user=%d iteration=%d", user.ID, iteration)); err != nil {
			slog.Error("loadgen dbtest failed", "user_id", user.ID, "error", err)
		}

		if err := client.createBooking(ctx, user.Token, user.PropertyID, iteration); err != nil {
			slog.Error("loadgen create booking failed", "user_id", user.ID, "error", err)
		}

		if err := client.listBookings(ctx, user.Token); err != nil {
			slog.Error("loadgen list bookings failed", "user_id", user.ID, "error", err)
		}

		if err := client.test(ctx); err != nil {
			slog.Error("loadgen test endpoint failed", "user_id", user.ID, "error", err)
		}

		jitter := time.Duration(rand.Intn(300)) * time.Millisecond

		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.Delay + jitter):
		}
	}
}

func waitPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	var lastErr error

	for attempt := 1; attempt <= 60; attempt++ {
		db, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			pingErr := db.Ping(ctx)
			if pingErr == nil {
				return db, nil
			}

			lastErr = pingErr
			db.Close()
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return nil, lastErr
}

func waitApp(ctx context.Context, client *apiClient) error {
	var lastErr error

	for attempt := 1; attempt <= 60; attempt++ {
		if err := client.test(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return lastErr
}

func getUserID(ctx context.Context, db *pgxpool.Pool, email string) (int64, error) {
	var id int64

	err := db.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func createProperty(ctx context.Context, db *pgxpool.Pool, ownerID int64, index int) (int64, error) {
	var id int64

	err := db.QueryRow(
		ctx,
		`INSERT INTO properties (owner_id, title, address, description, price_per_night)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		ownerID,
		fmt.Sprintf("Loadgen flat %d", index),
		fmt.Sprintf("Loadgen street %d", index),
		"Property generated by loadgen service",
		3000+index*100,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (c *apiClient) test(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/test", nil)
	if err != nil {
		return err
	}

	return c.doExpect(request, http.StatusOK)
}

func (c *apiClient) dbTest(ctx context.Context, message string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/dbtest", bytes.NewBufferString(message))
	if err != nil {
		return err
	}

	return c.doExpect(request, http.StatusCreated)
}

func (c *apiClient) register(ctx context.Context, email, password, fullName string) error {
	body := map[string]string{
		"email":     email,
		"password":  password,
		"full_name": fullName,
	}

	request, err := c.jsonRequest(ctx, http.MethodPost, "/auth/register", body)
	if err != nil {
		return err
	}

	return c.doExpect(request, http.StatusCreated)
}

func (c *apiClient) login(ctx context.Context, email, password string) (string, error) {
	body := map[string]string{
		"email":    email,
		"password": password,
	}

	request, err := c.jsonRequest(ctx, http.MethodPost, "/auth/login", body)
	if err != nil {
		return "", err
	}

	responseBody, err := c.do(request, http.StatusOK)
	if err != nil {
		return "", err
	}

	var response loginResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", err
	}

	if response.Token == "" {
		return "", fmt.Errorf("empty token")
	}

	return response.Token, nil
}

func (c *apiClient) createBooking(ctx context.Context, token string, propertyID int64, iteration int) error {
	start := time.Now().AddDate(0, 0, 10+iteration)
	end := start.AddDate(0, 0, 2)

	body := map[string]interface{}{
		"property_id": propertyID,
		"date_from":   start.Format("2006-01-02"),
		"date_to":     end.Format("2006-01-02"),
	}

	request, err := c.jsonRequest(ctx, http.MethodPost, "/bookings/create", body)
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", "Bearer "+token)

	return c.doExpect(request, http.StatusCreated)
}

func (c *apiClient) listBookings(ctx context.Context, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/bookings/list", nil)
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", "Bearer "+token)

	return c.doExpect(request, http.StatusOK)
}

func (c *apiClient) jsonRequest(ctx context.Context, method, path string, payload interface{}) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (c *apiClient) doExpect(request *http.Request, expectedStatus int) error {
	_, err := c.do(request, expectedStatus)
	return err
}

func (c *apiClient) do(request *http.Request, expectedStatus int) ([]byte, error) {
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != expectedStatus {
		return nil, fmt.Errorf("unexpected status %d, expected %d, body: %s", response.StatusCode, expectedStatus, string(body))
	}

	return body, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

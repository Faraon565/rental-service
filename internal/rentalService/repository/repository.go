package repository

import (
	"context"

	rentalService "rental-service/internal/rentalService"

	"github.com/jackc/pgx/v5/pgxpool"
)

type repo struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *repo {
	return &repo{
		db: db,
	}
}

func (r *repo) Init(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		full_name TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS properties (
		id BIGSERIAL PRIMARY KEY,
		owner_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		address TEXT NOT NULL,
		description TEXT,
		price_per_night NUMERIC(10, 2) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS booking_requests (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
		property_id BIGINT REFERENCES properties(id) ON DELETE CASCADE,
		date_from DATE NOT NULL,
		date_to DATE NOT NULL,
		status TEXT NOT NULL DEFAULT 'created',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS db_test_requests (
		id BIGSERIAL PRIMARY KEY,
		message TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	`

	_, err := r.db.Exec(ctx, query)
	return err
}

func (r *repo) GetTestMessage(ctx context.Context) (string, error) {
	return "Hello!", nil
}

func (r *repo) SaveDBTestMessage(ctx context.Context, message string) (int64, error) {
	var id int64

	err := r.db.QueryRow(
		ctx,
		`INSERT INTO db_test_requests (message) VALUES ($1) RETURNING id`,
		message,
	).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}

func (r *repo) CreateUser(ctx context.Context, user rentalService.User) error {
	query := `
		INSERT INTO users (email, password_hash, full_name)
		VALUES ($1, $2, $3)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		user.Email,
		user.PasswordHash,
		user.FullName,
	)

	return err
}

func (r *repo) GetUserByEmail(ctx context.Context, email string) (rentalService.User, error) {
	var user rentalService.User

	query := `
		SELECT id, email, password_hash, full_name
		FROM users
		WHERE email = $1
	`

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
	)

	if err != nil {
		return rentalService.User{}, err
	}

	return user, nil
}

func (r *repo) CreateProperty(ctx context.Context, req rentalService.CreatePropertyRequest) (int64, error) {
	var id int64

	query := `
		INSERT INTO properties (owner_id, title, address, description, price_per_night)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	err := r.db.QueryRow(
		ctx,
		query,
		req.OwnerID,
		req.Title,
		req.Address,
		req.Description,
		req.PricePerNight,
	).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}

func (r *repo) GetProperties(ctx context.Context) ([]rentalService.PropertyInfo, error) {
	query := `
		SELECT
			id,
			owner_id,
			title,
			address,
			COALESCE(description, '') AS description,
			TO_CHAR(price_per_night, 'FM999999990.00') AS price_per_night
		FROM properties
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	properties := make([]rentalService.PropertyInfo, 0)

	for rows.Next() {
		var property rentalService.PropertyInfo

		err = rows.Scan(
			&property.ID,
			&property.OwnerID,
			&property.Title,
			&property.Address,
			&property.Description,
			&property.PricePerNight,
		)
		if err != nil {
			return nil, err
		}

		properties = append(properties, property)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return properties, nil
}

func (r *repo) Close() {
	r.db.Close()
}

func (r *repo) CreateBooking(ctx context.Context, req rentalService.CreateBookingRequest) (int64, error) {
	var id int64

	query := `
		INSERT INTO booking_requests (user_id, property_id, date_from, date_to)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	err := r.db.QueryRow(
		ctx,
		query,
		req.UserID,
		req.PropertyID,
		req.DateFrom,
		req.DateTo,
	).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}

func (r *repo) GetBookingsByUserID(ctx context.Context, userID int64) ([]rentalService.BookingInfo, error) {
	query := `
		SELECT
			br.id,
			br.user_id,
			br.property_id,
			COALESCE(p.title, '') AS property_title,
			TO_CHAR(br.date_from, 'YYYY-MM-DD') AS date_from,
			TO_CHAR(br.date_to, 'YYYY-MM-DD') AS date_to,
			br.status,
			TO_CHAR(br.created_at, 'YYYY-MM-DD HH24:MI:SS') AS created_at
		FROM booking_requests br
		LEFT JOIN properties p ON p.id = br.property_id
		WHERE br.user_id = $1
		ORDER BY br.created_at DESC, br.id DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bookings := make([]rentalService.BookingInfo, 0)

	for rows.Next() {
		var booking rentalService.BookingInfo

		err = rows.Scan(
			&booking.ID,
			&booking.UserID,
			&booking.PropertyID,
			&booking.PropertyTitle,
			&booking.DateFrom,
			&booking.DateTo,
			&booking.Status,
			&booking.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		bookings = append(bookings, booking)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return bookings, nil
}

func (r *repo) UpdateBookingStatus(ctx context.Context, bookingID int64, status string) error {
	query := `
		UPDATE booking_requests
		SET status = $1
		WHERE id = $2
	`

	_, err := r.db.Exec(ctx, query, status, bookingID)
	return err
}

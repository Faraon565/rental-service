package rentalService

import (
	"context"
	"net/http"
)

type Handler interface {
	Test() http.HandlerFunc
	DBTest() http.HandlerFunc
	Register() http.HandlerFunc
	Login() http.HandlerFunc
	CreateBooking() http.HandlerFunc
	GetBookings() http.HandlerFunc

	UI() http.HandlerFunc
	UIRegister() http.HandlerFunc
	UILogin() http.HandlerFunc
	UILogout() http.HandlerFunc
	UICreateProperty() http.HandlerFunc
	UICreateBooking() http.HandlerFunc
}

type UseCase interface {
	GetTestMessage(ctx context.Context) (string, error)
	SaveDBTestMessage(ctx context.Context, message string) (int64, error)

	RegisterUser(ctx context.Context, req RegisterRequest) error
	LoginUser(ctx context.Context, req LoginRequest) (string, error)

	CreateProperty(ctx context.Context, req CreatePropertyRequest) (int64, error)
	GetProperties(ctx context.Context) ([]PropertyInfo, error)

	CreateBooking(ctx context.Context, req CreateBookingRequest) (int64, error)
	GetUserBookings(ctx context.Context, userID int64) ([]BookingInfo, error)
}

type Repository interface {
	GetTestMessage(ctx context.Context) (string, error)
	Init(ctx context.Context) error
	SaveDBTestMessage(ctx context.Context, message string) (int64, error)

	CreateUser(ctx context.Context, user User) error
	GetUserByEmail(ctx context.Context, email string) (User, error)

	CreateProperty(ctx context.Context, req CreatePropertyRequest) (int64, error)
	GetProperties(ctx context.Context) ([]PropertyInfo, error)

	CreateBooking(ctx context.Context, req CreateBookingRequest) (int64, error)
	GetBookingsByUserID(ctx context.Context, userID int64) ([]BookingInfo, error)
	UpdateBookingStatus(ctx context.Context, bookingID int64, status string) error

	Close()
}

type BookingPublisher interface {
	PublishBookingCreated(ctx context.Context, bookingID int64) error
	Close() error
}

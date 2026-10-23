package usecase

import (
	"context"
	"errors"
	"os"
	"time"

	rentalService "rental-service/internal/rentalService"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type useCase struct {
	repo      rentalService.Repository
	publisher rentalService.BookingPublisher
}

func New(repo rentalService.Repository, publisher rentalService.BookingPublisher) *useCase {
	return &useCase{
		repo:      repo,
		publisher: publisher,
	}
}

func (uc *useCase) GetTestMessage(ctx context.Context) (string, error) {
	return uc.repo.GetTestMessage(ctx)
}

func (uc *useCase) SaveDBTestMessage(ctx context.Context, message string) (int64, error) {
	return uc.repo.SaveDBTestMessage(ctx, message)
}

func (uc *useCase) RegisterUser(ctx context.Context, req rentalService.RegisterRequest) error {
	if req.Email == "" || req.Password == "" || req.FullName == "" {
		return errors.New("email, password and full_name are required")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := rentalService.User{
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		FullName:     req.FullName,
	}

	return uc.repo.CreateUser(ctx, user)
}

func (uc *useCase) LoginUser(ctx context.Context, req rentalService.LoginRequest) (string, error) {
	if req.Email == "" || req.Password == "" {
		return "", errors.New("email and password are required")
	}

	user, err := uc.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return "", errors.New("invalid email or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return "", errors.New("invalid email or password")
	}

	token, err := generateJWT(user.ID, user.Email)
	if err != nil {
		return "", err
	}

	return token, nil
}

func generateJWT(userID int64, email string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-key"
	}

	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func (uc *useCase) CreateProperty(ctx context.Context, req rentalService.CreatePropertyRequest) (int64, error) {
	if req.OwnerID <= 0 {
		return 0, errors.New("owner_id is required")
	}

	if req.Title == "" || req.Address == "" {
		return 0, errors.New("title and address are required")
	}

	if req.PricePerNight <= 0 {
		return 0, errors.New("price_per_night must be greater than zero")
	}

	return uc.repo.CreateProperty(ctx, req)
}

func (uc *useCase) GetProperties(ctx context.Context) ([]rentalService.PropertyInfo, error) {
	return uc.repo.GetProperties(ctx)
}

func (uc *useCase) CreateBooking(ctx context.Context, req rentalService.CreateBookingRequest) (int64, error) {
	if req.UserID <= 0 {
		return 0, errors.New("user_id is required")
	}

	if req.PropertyID <= 0 {
		return 0, errors.New("property_id is required")
	}

	if req.DateFrom == "" || req.DateTo == "" {
		return 0, errors.New("date_from and date_to are required")
	}

	dateFrom, err := time.Parse("2006-01-02", req.DateFrom)
	if err != nil {
		return 0, errors.New("date_from must be in YYYY-MM-DD format")
	}

	dateTo, err := time.Parse("2006-01-02", req.DateTo)
	if err != nil {
		return 0, errors.New("date_to must be in YYYY-MM-DD format")
	}

	if !dateTo.After(dateFrom) {
		return 0, errors.New("date_to must be after date_from")
	}

	bookingID, err := uc.repo.CreateBooking(ctx, req)
	if err != nil {
		return 0, err
	}

	if uc.publisher != nil {
		if err := uc.publisher.PublishBookingCreated(ctx, bookingID); err != nil {
			return 0, err
		}
	}

	return bookingID, nil
}

func (uc *useCase) GetUserBookings(ctx context.Context, userID int64) ([]rentalService.BookingInfo, error) {
	if userID <= 0 {
		return nil, errors.New("user_id is required")
	}

	return uc.repo.GetBookingsByUserID(ctx, userID)
}

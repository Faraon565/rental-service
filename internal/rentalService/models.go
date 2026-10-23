package rentalService

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	FullName     string
}

type CreatePropertyRequest struct {
	OwnerID       int64
	Title         string
	Address       string
	Description   string
	PricePerNight float64
}

type PropertyInfo struct {
	ID            int64  `json:"id"`
	OwnerID       int64  `json:"owner_id"`
	Title         string `json:"title"`
	Address       string `json:"address"`
	Description   string `json:"description"`
	PricePerNight string `json:"price_per_night"`
}

type CreateBookingRequest struct {
	UserID     int64  `json:"-"`
	PropertyID int64  `json:"property_id"`
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
}

type CreateBookingResponse struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

type BookingInfo struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	PropertyID    int64  `json:"property_id"`
	PropertyTitle string `json:"property_title,omitempty"`
	DateFrom      string `json:"date_from"`
	DateTo        string `json:"date_to"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type BookingCreatedEvent struct {
	BookingID int64 `json:"booking_id"`
}

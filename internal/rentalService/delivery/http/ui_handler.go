package http

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	rentalService "rental-service/internal/rentalService"
)

const authCookieName = "auth_token"

type uiPageData struct {
	IsAuthenticated bool
	UserID          int64
	Message         string
	Error           string
	Properties      []rentalService.PropertyInfo
	Bookings        []rentalService.BookingInfo
}

func (h *handler) UI() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		data := uiPageData{
			Message: r.URL.Query().Get("message"),
			Error:   r.URL.Query().Get("error"),
		}

		properties, err := h.uc.GetProperties(r.Context())
		if err != nil {
			data.Error = "failed to load properties: " + err.Error()
		} else {
			data.Properties = properties
		}

		userID, err := getUserIDFromCookie(r)
		if err == nil {
			data.IsAuthenticated = true
			data.UserID = userID

			bookings, err := h.uc.GetUserBookings(r.Context(), userID)
			if err != nil {
				data.Error = "failed to load bookings: " + err.Error()
			} else {
				data.Bookings = bookings
			}
		}

		renderUITemplate(w, data)
	}
}

func (h *handler) UIRegister() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			redirectUI(w, r, "", "failed to parse form")
			return
		}

		req := rentalService.RegisterRequest{
			Email:    strings.TrimSpace(r.FormValue("email")),
			Password: r.FormValue("password"),
			FullName: strings.TrimSpace(r.FormValue("full_name")),
		}

		if err := h.uc.RegisterUser(r.Context(), req); err != nil {
			redirectUI(w, r, "", err.Error())
			return
		}

		redirectUI(w, r, "user registered successfully", "")
	}
}

func (h *handler) UILogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			redirectUI(w, r, "", "failed to parse form")
			return
		}

		req := rentalService.LoginRequest{
			Email:    strings.TrimSpace(r.FormValue("email")),
			Password: r.FormValue("password"),
		}

		token, err := h.uc.LoginUser(r.Context(), req)
		if err != nil {
			redirectUI(w, r, "", err.Error())
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     authCookieName,
			Value:    token,
			Path:     "/ui",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   24 * 60 * 60,
		})

		redirectUI(w, r, "login completed successfully", "")
	}
}

func (h *handler) UILogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     authCookieName,
			Value:    "",
			Path:     "/ui",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})

		redirectUI(w, r, "logout completed successfully", "")
	}
}

func (h *handler) UICreateProperty() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, err := getUserIDFromCookie(r)
		if err != nil {
			redirectUI(w, r, "", "login is required")
			return
		}

		if err := r.ParseForm(); err != nil {
			redirectUI(w, r, "", "failed to parse form")
			return
		}

		price, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("price_per_night")), 64)
		if err != nil {
			redirectUI(w, r, "", "price_per_night must be a number")
			return
		}

		req := rentalService.CreatePropertyRequest{
			OwnerID:       userID,
			Title:         strings.TrimSpace(r.FormValue("title")),
			Address:       strings.TrimSpace(r.FormValue("address")),
			Description:   strings.TrimSpace(r.FormValue("description")),
			PricePerNight: price,
		}

		if _, err := h.uc.CreateProperty(r.Context(), req); err != nil {
			redirectUI(w, r, "", err.Error())
			return
		}

		redirectUI(w, r, "property created successfully", "")
	}
}

func (h *handler) UICreateBooking() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, err := getUserIDFromCookie(r)
		if err != nil {
			redirectUI(w, r, "", "login is required")
			return
		}

		if err := r.ParseForm(); err != nil {
			redirectUI(w, r, "", "failed to parse form")
			return
		}

		propertyID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("property_id")), 10, 64)
		if err != nil {
			redirectUI(w, r, "", "property_id must be a number")
			return
		}

		req := rentalService.CreateBookingRequest{
			UserID:     userID,
			PropertyID: propertyID,
			DateFrom:   strings.TrimSpace(r.FormValue("date_from")),
			DateTo:     strings.TrimSpace(r.FormValue("date_to")),
		}

		if _, err := h.uc.CreateBooking(r.Context(), req); err != nil {
			redirectUI(w, r, "", err.Error())
			return
		}

		redirectUI(w, r, "booking request created successfully", "")
	}
}

func renderUITemplate(w http.ResponseWriter, data uiPageData) {
	tmpl, err := template.ParseFiles("web/templates/ui.html")
	if err != nil {
		http.Error(w, "failed to parse ui template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "failed to render ui template", http.StatusInternalServerError)
	}
}

func redirectUI(w http.ResponseWriter, r *http.Request, message, errorMessage string) {
	values := url.Values{}
	if message != "" {
		values.Set("message", message)
	}
	if errorMessage != "" {
		values.Set("error", errorMessage)
	}

	location := "/ui"
	if encoded := values.Encode(); encoded != "" {
		location += "?" + encoded
	}

	http.Redirect(w, r, location, http.StatusSeeOther)
}

func getUserIDFromCookie(r *http.Request) (int64, error) {
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		return 0, errors.New("auth cookie is required")
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return []byte(getJWTSecret()), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("invalid auth cookie")
	}

	userIDValue, ok := claims["user_id"]
	if !ok {
		return 0, errors.New("user_id not found in token")
	}

	userID, ok := userIDValue.(float64)
	if !ok || userID <= 0 {
		return 0, errors.New("invalid user_id in token")
	}

	return int64(userID), nil
}

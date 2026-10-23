FROM golang:latest AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/rental-app ./cmd/app

FROM alpine:latest

WORKDIR /app
COPY --from=builder /bin/rental-app /bin/rental-app
COPY web ./web

EXPOSE 8080
CMD ["/bin/rental-app"]

# ---- Build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server/

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12

COPY --from=builder /server /server

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/server"]

# ---- Build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -o /healthcheck ./cmd/healthcheck/

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12

COPY --from=builder /server /server
COPY --from=builder /healthcheck /healthcheck

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/healthcheck"]

ENTRYPOINT ["/server"]

FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/smart-playlist ./cmd/app

FROM alpine:3.21

RUN adduser -D -H -s /sbin/nologin appuser

WORKDIR /app

COPY --from=builder /out/smart-playlist /app/smart-playlist

USER appuser

ENTRYPOINT ["/app/smart-playlist"]

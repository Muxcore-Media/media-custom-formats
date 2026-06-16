FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY media-custom-formats/ /build/media-custom-formats/
WORKDIR /build/media-custom-formats
RUN go mod download && CGO_ENABLED=0 go build -o /media-custom-formats ./cmd/module
FROM alpine:3.21
RUN adduser -D -h /data app
USER app
WORKDIR /app
COPY --from=builder /media-custom-formats .
ENTRYPOINT ["./media-custom-formats"]

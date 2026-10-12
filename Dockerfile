FROM golang:1.27-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/community-waste-api ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S appgroup \
    && adduser -S -G appgroup -h /app appuser \
    && mkdir -p /app/uploads
WORKDIR /app
COPY --from=builder --chown=appuser:appgroup /out/community-waste-api ./community-waste-api
COPY --from=builder --chown=appuser:appgroup /src/migrations ./migrations
COPY --from=builder --chown=appuser:appgroup /src/uploads/.gitkeep ./uploads/.gitkeep
USER appuser:appgroup
EXPOSE 8080
ENTRYPOINT ["./community-waste-api"]

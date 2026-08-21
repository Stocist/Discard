# Stage 1: Build frontend
FROM node:24.19.0-alpine3.23 AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm install --global npm@11.17.0 && npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1.27.0-alpine3.23 AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/build ./internal/frontend/build
RUN CGO_ENABLED=0 go build -o discard ./cmd/discard

# Stage 3: Minimal runtime
FROM alpine:3.24.1
RUN apk add --no-cache ca-certificates ffmpeg libwebp-tools yt-dlp
WORKDIR /app
COPY --from=backend /app/discard /app/discard
EXPOSE 4000
CMD ["/app/discard"]

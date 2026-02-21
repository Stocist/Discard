# Stage 1: Build Go binary
FROM golang:1.23-alpine AS builder-backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /app/discard ./cmd/discard

# Stage 2: Build SvelteKit frontend
FROM node:22-alpine AS builder-frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 3: Minimal runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates ffmpeg yt-dlp
WORKDIR /app
COPY --from=builder-backend /app/discard /app/discard
COPY --from=builder-frontend /src/build /app/web/build
EXPOSE 4000
ENTRYPOINT ["/app/discard"]

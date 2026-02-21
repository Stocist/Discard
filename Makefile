.PHONY: dev-backend dev-frontend build-backend build-frontend build docker-up docker-down clean

dev-backend:
	go run ./cmd/discard

dev-frontend:
	cd web && npm run dev

build-backend:
	go build -o bin/discard ./cmd/discard

build-frontend:
	cd web && npm run build

build: build-backend build-frontend

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -rf bin/ web/build/

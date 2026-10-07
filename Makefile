.PHONY: build web server run dev-api dev-web seed check clean

build: web server

web:
	cd web && npm ci && npm run build

server:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/portfolio ./cmd/portfolio

run: build
	./bin/portfolio

# Two terminals: `make dev-api` (Go on :8080) and `make dev-web` (Vite on :5173,
# proxying /api and /media). The dashboard is at http://localhost:5173/__admin.
dev-api:
	go run ./cmd/portfolio

dev-web:
	cd web && npm run dev

# Load seed/data.json into an empty database. Add CV=path/to/cv.pdf to upload
# a CV, FORCE=1 to replace existing projects, experience and skills.
seed:
	go run ./cmd/portfolio seed $(if $(CV),-cv $(CV)) $(if $(FORCE),-force)

check:
	go vet ./...
	cd web && npm run typecheck

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg

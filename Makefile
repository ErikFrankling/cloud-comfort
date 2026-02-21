.PHONY: dev-backend dev-frontend install build

dev-backend:
	cd backend && go run .

dev-frontend:
	cd frontend && npm run dev

install:
	cd frontend && npm install

build:
	cd backend && go build -o cloud-comfort .
	cd frontend && npm run build

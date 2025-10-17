.PHONY: help migrate-up migrate-down migrate-create sqlc-generate db-setup install-tools

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install-tools: ## Install migration and sqlc tools
	@echo "Installing golang-migrate..."
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "Installing sqlc..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "✅ Tools installed successfully"

migrate-create: ## Create a new migration file (usage: make migrate-create name=your_migration_name)
	@if [ -z "$(name)" ]; then \
		echo "Error: name parameter is required"; \
		echo "Usage: make migrate-create name=your_migration_name"; \
		exit 1; \
	fi
	@migrate create -ext sql -dir db/migrations -seq $(name)
	@echo "✅ Migration files created"

migrate-up: ## Run all pending migrations
	@echo "Running migrations..."
	@migrate -path db/migrations -database "$(DATABASE_URL)" -verbose up
	@echo "✅ Migrations completed"

migrate-down: ## Rollback last migration
	@echo "Rolling back last migration..."
	@migrate -path db/migrations -database "$(DATABASE_URL)" -verbose down 1
	@echo "✅ Rollback completed"

migrate-force: ## Force migration version (usage: make migrate-force version=1)
	@if [ -z "$(version)" ]; then \
		echo "Error: version parameter is required"; \
		echo "Usage: make migrate-force version=1"; \
		exit 1; \
	fi
	@migrate -path db/migrations -database "$(DATABASE_URL)" force $(version)
	@echo "✅ Migration version forced to $(version)"

migrate-version: ## Show current migration version
	@migrate -path db/migrations -database "$(DATABASE_URL)" version

sqlc-generate: ## Generate Go code from SQL queries using sqlc
	@echo "Generating Go code from SQL..."
	@sqlc generate
	@echo "✅ Code generation completed"

db-setup: migrate-up sqlc-generate ## Setup database (run migrations and generate code)
	@echo "✅ Database setup completed"

# Build targets
build: sqlc-generate ## Build the application
	@echo "Building application..."
	@go build -o paybutton .
	@echo "✅ Build completed"

run: ## Run the application
	@go run main.go

test: ## Run tests
	@go test -v ./...

clean: ## Clean build artifacts
	@rm -f paybutton paybutton_* test_build*
	@echo "✅ Cleaned build artifacts"

# Database connection string check
check-db: ## Verify DATABASE_URL is set
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "❌ Error: DATABASE_URL environment variable is not set"; \
		echo "Example: export DATABASE_URL='postgresql://user:password@host:5432/dbname?sslmode=disable'"; \
		exit 1; \
	else \
		echo "✅ DATABASE_URL is set"; \
	fi

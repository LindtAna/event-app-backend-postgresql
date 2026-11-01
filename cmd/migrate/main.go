package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("Please provide a migration direction: 'up' or 'down' ")
	}

	direction := os.Args[1]

	if direction != "up" && direction != "down" {
		return fmt.Errorf("invalid direction %q: use 'up' or 'down'", direction)
	}

	databaseURL := strings.TrimSpace(
		os.Getenv("MIGRATIONS_DATABASE_URL"),
	)
	if databaseURL == "" {
		return fmt.Errorf("MIGRATIONS_DATABASE_URL must be set")
	}

	db, err := sql.Open("pgx", databaseURL)

	if err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL connection: %w", err)
	}

	defer db.Close()

	// Verify the database connection before running migrations.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Initialize the PostgreSQL migration driver.
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL migration driver: %w", err)
	}

	// Load SQL migration files.
	source, err := (&file.File{}).Open("cmd/migrate/migrations")
	if err != nil {
		return fmt.Errorf("failed to open migration files: %w", err)
	}
	defer source.Close()

	// Create the migration instance.
	m, err := migrate.NewWithInstance("file", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	defer m.Close()

	switch direction {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to apply migrations: %w", err)
		}
		log.Println("Migrations applied successfully.")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to roll back migrations: %w", err)
		}
		log.Println("Migrations rolled back successfully.")
	}

	return nil
}

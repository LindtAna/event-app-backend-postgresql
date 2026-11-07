//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"event-app/internal/database"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

const integrationJWTSecret = "integration-test-secret-that-is-at-least-32-bytes"

type integrationTestEnvironment struct {
	app    *application
	server *httptest.Server
	db     *sql.DB
}

// integrationProjectRoot ermittelt das Projektverzeichnis anhand des Dateipfads
// und des aktuellen Arbeitsverzeichnisses.
func integrationProjectRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to determine the integration test file path.")
	}

	candidates := []string{filepath.Dir(filename)}

	if workingDir, err := os.Getwd(); err == nil {
		candidates = append(candidates, workingDir)
	}

	for _, candidate := range candidates {
		dir, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}

		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}

			dir = parent
		}
	}

	t.Fatal("Failed to locate the project root (go.mod).")
	return ""
}

// applyIntegrationMigrations wendet alle noch ausstehenden Datenbankmigrationen an.
// applyIntegrationMigrations wendet alle noch ausstehenden Datenbankmigrationen an.
func applyIntegrationMigrations(databaseURL, projectRoot string) error {
	// Eine separate Datenbankverbindung für Migrationen verwenden,
	// damit die Verbindung der Anwendung nicht geschlossen wird.
	migrationDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to initialize migration database connection: %w", err)
	}
	defer migrationDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := migrationDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to migration database: %w", err)
	}

	migrationsPath := filepath.Join(projectRoot, "cmd", "migrate", "migrations")

	migrationSource, err := (&file.File{}).Open(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to open migration files: %w", err)
	}

	driver, err := postgres.WithInstance(migrationDB, &postgres.Config{})
	if err != nil {
		_ = migrationSource.Close()
		return fmt.Errorf("failed to initialize PostgreSQL migration driver: %w", err)
	}

	migration, err := migrate.NewWithInstance(
		"file",
		migrationSource,
		"postgres",
		driver,
	)
	if err != nil {
		_ = migrationSource.Close()
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	defer migration.Close()

	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply integration test migrations: %w", err)
	}

	return nil
}

// newIntegrationTestEnvironment erstellt eine Testumgebung mit einer echten PostgreSQL-Datenbank.
func newIntegrationTestEnvironment(t *testing.T) *integrationTestEnvironment {
	t.Helper()

	projectRoot := integrationProjectRoot(t)

	// Die lokale .env-Datei laden, ohne bereits gesetzte Umgebungsvariablen zu überschreiben.
	envPath := filepath.Join(projectRoot, ".env")

	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(envPath); err != nil {
			t.Fatalf("Failed to load .env file: %v", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Failed to access .env file: %v", err)
	}

	// Ausschließlich die dedizierte Testdatenbank verwenden.
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL must be set. No fallback database URL is allowed.")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("Failed to initialize the test database connection: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close the test database connection: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to connect to the test database: %v", err)
	}

	if err := applyIntegrationMigrations(databaseURL, projectRoot); err != nil {
		t.Fatalf("Failed to prepare the test database: %v", err)
	}

	app := &application{
		jwtSecret:    integrationJWTSecret,
		clientOrigin: "http://localhost:5173",
		db:           db,
		models:       database.NewModels(db),
	}

	server := httptest.NewServer(app.routes())

	// Den HTTP-Testserver vor dem Schließen der Datenbank beenden.
	t.Cleanup(server.Close)

	return &integrationTestEnvironment{
		app:    app,
		server: server,
		db:     db,
	}
}

// dekodiert den JSON-Body einer HTTP-Antwort
func decodeIntegrationResponse(
	response *http.Response,
	target interface{},
) error {
	return json.NewDecoder(response.Body).Decode(target)
}

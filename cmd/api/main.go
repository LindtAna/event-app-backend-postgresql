package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	_ "event-app/docs"
	"event-app/internal/database"
	"event-app/internal/env"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
)

// @title EventApp Backend API
// @version 1.0
// @description Tenant-scoped event management API built with Go and Gin.
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter your bearer token in the format **Bearer &lt;token&gt;**
type application struct {
	port         int
	jwtSecret    string
	clientOrigin string
	models       database.Models
}

func main() {
	jwtSecret := env.GetEnvString("JWT_SECRET", "")
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be set and contain at least 32 bytes")
	}

	databaseURL := strings.TrimSpace(
		env.GetEnvString("DATABASE_URL", ""),
	)
	if databaseURL == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	// PostgreSQL connection via pgx and database/sql.
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize PostgreSQL connection: %v", err)
	}

	// Verify the database connection during application startup.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	defer db.Close()

	models := database.NewModels(db)
	app := &application{
		port:         env.GetEnvInt("PORT", 8080),
		jwtSecret:    jwtSecret,
		clientOrigin: env.GetEnvString("CLIENT_ORIGIN", "http://localhost:5173"),
		models:       models,
	}

	if err := app.serve(); err != nil {
		log.Fatal(err)
	}
}

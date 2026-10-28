package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Please provide a migration direction: 'up' or 'down' ")
	}

	direction := os.Args[1]

	db, err := sql.Open("sqlite", "./data.db")

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	instance, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		log.Fatal(err)
	}

	fSrc, err := (&file.File{}).Open("cmd/migrate/migrations")

	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.NewWithInstance("file", fSrc, "sqlite", instance)

	if err != nil {
		log.Fatal(err)
	}

	switch direction {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}

	default:
		log.Fatal("Invalid direction. Use 'up' or 'down")
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// serve startet den HTTP-Server und verwaltet dessen
// kontrolliertes Herunterfahren (Graceful Shutdown)
func (app *application) serve() error {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", app.port),
		Handler:           app.routes(),
		IdleTimeout:       time.Minute,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // Maximale Wartezeit auf vollständige HTTP-Header
		WriteTimeout:      30 * time.Second,
	}

	// Gepufferter Channel zur Übermittlung von Serverfehlern aus der nachfolgend gestarteten Goroutine
	serverErrors := make(chan error, 1)

	go func() {
		err := server.ListenAndServe()

		// Ein reguläres Herunterfahren über Shutdown()
		// führt zu http.ErrServerClosed und gilt nicht als Fehler
		if errors.Is(err, http.ErrServerClosed) {
			serverErrors <- nil
			return
		}

		// Unerwartete Fehler beim Starten oder Betreiben
		// des Servers werden an die Hauptfunktion weitergegeben
		serverErrors <- err
	}()

	// Channel zum Empfang von Betriebssystemsignalen
	quit := make(chan os.Signal, 1)

	// SIGINT: Beendigung, (zB Ctrl+C)
	// SIGTERM: Anforderung zum Beenden durch die Hosting-Plattform
	signal.Notify(
		quit,
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer signal.Stop(quit)

	log.Printf("Starting server on port :%d", app.port)

	// Warten, bis entweder ein Serverfehler auftritt
	// oder ein Signal zum Herunterfahren empfangen wird
	select {
	case err := <-serverErrors:
		return err

	case sig := <-quit:
		log.Printf("Received signal %s. Shutting down...", sig)

		// Der Server erhält maximal zehn Sekunden,
		// um laufende HTTP-Anfragen ordnungsgemäß abzuschließen
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		// Shutdown stoppt die Annahme neuer Verbindungen
		// und wartet auf den Abschluss aktiver Requests
		if err := server.Shutdown(ctx); err != nil {
			// Falls das reguläre Herunterfahren nicht innerhalb des Timeouts abgeschlossen werden kann, werden verbleibende Verbindungen zwangsweise geschlossen
			_ = server.Close()

			return fmt.Errorf(
				"graceful shutdown failed: %w",
				err,
			)
		}

		log.Println("Server stopped gracefully")
		return nil
	}
}

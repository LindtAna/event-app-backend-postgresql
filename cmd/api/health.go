package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// überprüft, ob die Anwendung bereit ist, HTTP-Anfragen zu verarbeiten
// Dafür wird geprüft, ob eine Verbindung zur PostgreSQL-DB hergestellt werden kann
// Der Handler wird von externen Plattformen (zB Render) verwendet,
// um die Betriebsbereitschaft des Backend-Services zu überwachen
// Die Route benötigt keine Authentifizierung
func (app *application) healthReady(c *gin.Context) {
	if app.db == nil {
		// Ohne initialisierte DB-verbindung kann die App keine datenbankabhängigen Anfragen verarbeiten
		// Der Service gilt als nicht bereit. Interne Fehlerdetails werden nicht an den externen Aufrufer weitergegeben(public endpoint!)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
		})
		return
	}
	// Die Datenbankprüfung erhält ein eigenes Timeout
	// Dadurch wird verhindert, dass der Health Check bei einer nicht erreichbaren DB unbegrenzt auf eine Antwort wartet
	ctx, cancel := context.WithTimeout(
		c.Request.Context(),
		2*time.Second,
	)
	defer cancel()

	// PingContext prüft, ob über den vorhandenen DB-Pool eine Verbindung zu PostgreSQL hergestellt werden kann
	if err := app.db.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
		})
		return
	}

	// Signalisiert, dass der Backend-Service erreichbar und die DB-Verbindung verfügbar ist
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

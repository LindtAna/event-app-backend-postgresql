package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (app *application) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.Fields(authHeader)

		if len(parts) != 2 ||
			!strings.EqualFold(parts[0], "Bearer") ||
			parts[1] == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "A Bearer access token is required",
			})
			c.Abort()
			return
		}
		// Refresh-Token wird hier zurückgewiesen, da nur Access-Tokens
		// für geschützte API-Endpunkte zugelassen sind.
		claims, err := app.parseAuthToken(parts[1], accessTokenType)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired access token",
			})
			c.Abort()
			return
		}

		// Benutzerdaten bei jedem geschützten Request neu aus der DB laden.
		// TenantID und Rolle werden nicht aus dem Token übernommen.
		user, err := app.models.Users.Get(
			c.Request.Context(),
			claims.UserID)
		if err != nil {
			log.Printf("Authenticated user lookup failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not verify account",
			})
			c.Abort()
			return
		}

		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Account is no longer available",
			})
			c.Abort()
			return
		}

		active, err := app.isActiveUserAndTenant(
			c.Request.Context(),
			user,
		)
		if err != nil {
			log.Printf("Tenant status verification failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not verify account",
			})
			c.Abort()
			return
		}

		if !active {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Account is unavailable",
			})
			c.Abort()
			return
		}

		// Vertrauenswürdiger Kontext für Handler und nachfolgende Middleware.
		c.Set("user", user)
		c.Set("userId", user.Id)
		c.Set("tenantId", user.TenantID)
		c.Set("role", user.Role)

		c.Next()
	}
}

// isAllowedOrigin vergleicht den Origin-Header mit der konfigurierten Allowlist.
// Mehrere Frontend-Origins können in app.clientOrigin kommasepariert stehen.
func (app *application) isAllowedOrigin(origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return false
	}

	for _, configuredOrigin := range strings.Split(app.clientOrigin, ",") {
		configuredOrigin = strings.TrimRight(
			strings.TrimSpace(configuredOrigin),
			"/",
		)

		if configuredOrigin != "" &&
			strings.EqualFold(origin, configuredOrigin) {
			return true
		}
	}

	return false
}

// CORSMiddleware erlaubt nur konfigurierte Frontend-Origins.
func (app *application) CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.Request.Header.Get("Origin"))

		if origin != "" {
			if !app.isAllowedOrigin(origin) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "Origin not allowed",
				})
				return
			}

			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Add("Vary", "Origin")
			c.Writer.Header().Set(
				"Access-Control-Allow-Headers",
				"Content-Type, Content-Length, Accept-Encoding, "+
					"X-CSRF-Token, Authorization, Accept, Origin, "+
					"Cache-Control, X-Requested-With",
			)
			c.Writer.Header().Set(
				"Access-Control-Allow-Methods",
				"POST, OPTIONS, GET, PUT, DELETE, PATCH",
			)
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

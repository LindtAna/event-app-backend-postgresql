package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (app *application) routes() http.Handler {
	g := gin.Default()

	g.Use(app.CORSMiddleware())

	// Public routes: no access token required.
	v1 := g.Group("/api/v1")
	{
		v1.POST("/auth/register", app.registerUser)
		v1.POST("/auth/login", app.login)
		v1.POST("/auth/refresh", app.refreshToken)
		v1.POST("/auth/logout", app.logout)
	}

	// Protected routes: valid access token and active tenant required
	protected := v1.Group("/")
	protected.Use(app.AuthMiddleware())

	{
		// Invitations (owner only)
		ownerProtected := protected.Group("/")
		ownerProtected.Use(app.OwnerOnlyMiddleware())

		ownerProtected.POST("/invitations", app.createInvitation)
		ownerProtected.GET("/invitations", app.getAllInvitations)
		ownerProtected.DELETE("/invitations/:id", app.revokeInvitation)
		ownerProtected.DELETE("/invitations/:id/permanent", app.deleteInvitation)

		// User profile
		protected.PUT("/users/profile", app.updateProfile)

		// Events
		protected.GET("/events", app.getAllEvents)
		protected.GET("/events/:id", app.getEvent)
		protected.POST("/events", app.createEvent)
		protected.PUT("/events/:id", app.updateEvent)
		protected.DELETE("/events/:id", app.deleteEvent)

		// Categories
		protected.GET("/categories", app.getAllCategories)

		// Event attendees
		protected.GET("/events/:id/attendees", app.getAttendeesForEvent)
		protected.POST("/events/:id/attendees/:userId", app.addAttendeeToEvent)
		protected.DELETE("/events/:id/attendees/:userId", app.deleteAttendeeFromEvent)

		// Events associated with a user
		protected.GET("/attendees/:id/events", app.getEventsByAttendee)
	}

	// Swagger documentation
	g.GET("/swagger/*any", func(c *gin.Context) {
		if c.Request.RequestURI == "/swagger" {
			c.Redirect(302, "/swagger/index.html")
		}
		ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.URL("http://localhost:8080/swagger/doc.json"))(c)
	})

	return g
}

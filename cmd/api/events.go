package main

import (
	"database/sql"
	"errors"
	"event-app/internal/database"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func eventRequestScope(c *gin.Context) (userID, tenantID int, ok bool) {
	userValue, userExists := c.Get("userId")
	tenantValue, tenantExists := c.Get("tenantId")

	if !userExists || !tenantExists {
		return 0, 0, false
	}

	userID, userOK := userValue.(int)
	tenantID, tenantOK := tenantValue.(int)

	if !userOK || !tenantOK || userID <= 0 || tenantID <= 0 {
		return 0, 0, false
	}

	return userID, tenantID, true
}

func parsePositiveID(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}

// CreateEvent creates a new event
//
// @Summary Creates an event in the authenticated tenant
// @Tags events
// @Accept json
// @Produce json
// @Param event body database.Event true "Event"
// @Success 201 {object} database.Event
// @Router /api/v1/events [post]
// @Security BearerAuth
func (app *application) createEvent(c *gin.Context) {
	userID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var event database.Event
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Never trust OwnerId supplied by the client.
	event.OwnerId = userID

	err := app.models.Events.CreateWithCategory(
		c.Request.Context(),
		tenantID,
		&event,
	)
	if err != nil {
		if errors.Is(err, database.ErrInvalidCategory) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Please select a category belonging to your company or provide a category name",
			})
			return
		}

		if errors.Is(err, database.ErrInvalidEventScope) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create event",
		})
		return
	}

	c.JSON(http.StatusCreated, event)
}

// getAllEvents returns all events or filters them by ownerId
//
// @Summary Returns events belonging to the authenticated tenant
// @Tags events
// @Produce json
// @Param ownerId query int false "Filter by event owner within the tenant"
// @Success 200 {array} database.Event
// @Router /api/v1/events [get]
// @Security BearerAuth
func (app *application) getAllEvents(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var (
		events []*database.Event
		err    error
	)

	ownerIDParam := c.Query("ownerId")

	if ownerIDParam != "" {
		ownerID, parseErr := parsePositiveID(ownerIDParam)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid ownerId parameter",
			})
			return
		}
		events, err = app.models.Events.GetByOwnerID(
			c.Request.Context(),
			tenantID,
			ownerID,
		)
	} else {
		events, err = app.models.Events.GetAll(
			c.Request.Context(),
			tenantID,
		)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve events"})
		return
	}

	c.JSON(http.StatusOK, events)
}

// GetEvent returns a single event
//
// @Summary Returns an event belonging to the authenticated tenant
// @Tags events
// @Produce json
// @Param id path int true "Event ID"
// @Success 200 {object} database.Event
// @Router /api/v1/events/{id} [get]
// @Security BearerAuth
func (app *application) getEvent(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := parsePositiveID(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event id"})
		return
	}

	event, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve event"})
		return
	}

	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// UpdateEvent updates an existing event
//
// @Summary Updates an event created by the authenticated user
// @Tags events
// @Accept json
// @Produce json
// @Param id path int true "Event ID"
// @Param event body database.Event true "Updated event"
// @Success 200 {object} database.Event
// @Router /api/v1/events/{id} [put]
// @Security BearerAuth
func (app *application) updateEvent(c *gin.Context) {
	userID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	id, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event id"})
		return
	}
	existingEvent, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve event"})
		return
	}

	if existingEvent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	if existingEvent.OwnerId != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorised to update this event"})
		return
	}

	var updatedEvent database.Event
	if err := c.ShouldBindJSON(&updatedEvent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ID and owner always come from the path and authenticated context.
	updatedEvent.Id = id
	updatedEvent.OwnerId = userID

	err = app.models.Events.UpdateWithCategory(
		c.Request.Context(),
		tenantID,
		userID,
		&updatedEvent,
	)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrInvalidCategory):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Please select a category belonging to your company or provide a category name",
			})
		case errors.Is(err, sql.ErrNoRows):
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to update event",
			})
		}
		return
	}
	c.JSON(http.StatusOK, updatedEvent)
}

// DeleteEvent deletes an existing event
//
// @Summary Deletes an event created by the authenticated user
// @Tags events
// @Param id path int true "Event ID"
// @Success 204
// @Router /api/v1/events/{id} [delete]
// @Security BearerAuth
func (app *application) deleteEvent(c *gin.Context) {
	userID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := parsePositiveID(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event id"})
		return
	}

	event, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve event",
		})
		return
	}

	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	if event.OwnerId != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You are not authorised to delete this event",
		})
		return
	}

	err = app.models.Events.DeleteWithCategory(
		c.Request.Context(),
		tenantID,
		userID,
		id,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete event",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

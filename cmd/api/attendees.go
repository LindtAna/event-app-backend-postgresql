package main

import (
	"database/sql"
	"errors"
	"event-app/internal/database"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AddAttendeeToEvent adds an attendee to an event
// @Summary Adds a user from the same tenant to an event
// @Tags attendees
// @Param id path int true "Event ID"
// @Param userId path int true "User ID"
// @Success 201 {object} database.Attendee
// @Router /api/v1/events/{id}/attendees/{userId} [post]
// @Security BearerAuth
func (app *application) addAttendeeToEvent(c *gin.Context) {
	currentUserID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	eventID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event Id"})
		return
	}

	targetUserID, err := parsePositiveID(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	event, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		eventID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retreive event"})
		return
	}

	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	targetUser, err := app.models.Users.GetByIDInTenant(
		c.Request.Context(),
		tenantID,
		targetUserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retreive user"})
		return
	}

	if targetUser == nil || targetUser.Status != "active" {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if event.OwnerId != currentUserID && targetUserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You are not authorised to add an attendee to this event",
		})
		return
	}

	existing, err := app.models.Attendees.GetByEventAndAttendee(
		c.Request.Context(),
		tenantID,
		eventID,
		targetUserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retreive attendee"})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Attendee already exists"})
		return
	}

	attendee := &database.Attendee{
		EventId: eventID,
		UserId:  targetUserID,
	}

	err = app.models.Attendees.Insert(
		c.Request.Context(),
		tenantID,
		attendee,
	)
	if err != nil {
		if errors.Is(err, database.ErrAttendeeAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "Attendee already exists"})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Event or user not found in this company",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to add attendee",
		})
		return
	}

	c.JSON(http.StatusCreated, attendee)
}

// GetAttendeesForEvent returns all attendees for a given event
//
// @Summary Lists attendees for an event in the authenticated tenant
// @Tags attendees
// @Param id path int true "Event ID"
// @Success 200 {array} database.User
// @Router /api/v1/events/{id}/attendees [get]
// @Security BearerAuth
func (app *application) getAttendeesForEvent(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	eventID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event Id"})
		return
	}

	event, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		eventID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve event"})
		return
	}
	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}
	users, err := app.models.Attendees.GetAttendeesByEvent(
		c.Request.Context(),
		tenantID,
		eventID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve attendees",
		})
		return
	}

	c.JSON(http.StatusOK, users)
}

// DeleteAttendeeFromEvent deletes an attendee from an event
// @Summary Removes an attendee from an event
// @Tags attendees
// @Param id path int true "Event ID"
// @Param userId path int true "User ID"
// @Success 204
// @Router /api/v1/events/{id}/attendees/{userId} [delete]
// @Security BearerAuth
func (app *application) deleteAttendeeFromEvent(c *gin.Context) {
	currentUserID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	eventID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event Id"})
		return
	}

	targetUserID, err := parsePositiveID(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user Id"})
		return
	}

	event, err := app.models.Events.Get(
		c.Request.Context(),
		tenantID,
		eventID,
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

	if event.OwnerId != currentUserID && targetUserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You are not authorised to remove this attendee",
		})
		return
	}

	err = app.models.Attendees.Delete(
		c.Request.Context(),
		tenantID,
		targetUserID,
		eventID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Attendee not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to remove attendee",
		})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// GetEventsByAttendee returns all events for a given attendee
//
// @Summary Lists events for a user in the authenticated tenant
// @Tags attendees
// @Param id path int true "User ID"
// @Success 200 {array} database.Event
// @Router /api/v1/attendees/{id}/events [get]
// @Security BearerAuth
func (app *application) getEventsByAttendee(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	attendeeID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid attendee Id"})
		return
	}

	user, err := app.models.Users.GetByIDInTenant(
		c.Request.Context(),
		tenantID,
		attendeeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve user",
		})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	events, err := app.models.Attendees.GetEventsByAttendee(
		c.Request.Context(),
		tenantID,
		attendeeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get events"})
		return
	}

	c.JSON(http.StatusOK, events)
}

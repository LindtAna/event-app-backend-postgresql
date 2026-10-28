package main

import (
	"errors"
	"event-app/internal/database"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type createInvitationRequest struct {
	ExpiresAt string `json:"expiresAt" binding:"required"`
}

// OwnerOnlyMiddleware erlaubt den Zugriff ausschließlich für Tenant-Owner
func (app *application) OwnerOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleValue, exists := c.Get("role")
		role, ok := roleValue.(string)

		if !exists || !ok || role == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		if role != "owner" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Only the tenant owner can manage invitations",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// createInvitation erstellt eine Einladung für den authentifizierten Tenant.
//
// @Summary Erstellt eine Einladung
// @Tags invitations
// @Accept json
// @Produce json
// @Param invitation body createInvitationRequest true "Ablaufzeitpunkt der Einladung"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/invitations [post]
func (app *application) createInvitation(c *gin.Context) {
	userID, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	var req createInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid invitation data",
		})
		return
	}

	// Der Ablaufzeitpunkt muss ein gültiger RFC3339-Zeitstempel sein.
	expiresAt, err := time.Parse(
		time.RFC3339,
		strings.TrimSpace(req.ExpiresAt),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "expiresAt must be a valid RFC3339 timestamp",
		})
		return
	}

	if !expiresAt.After(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "expiresAt must be in the future",
		})
		return
	}

	invitation, rawToken, err := app.models.Invitations.Create(
		c.Request.Context(),
		tenantID,
		userID,
		expiresAt,
	)
	if err != nil {
		if errors.Is(err, database.ErrInvalidInvitation) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid invitation data",
			})
			return
		}

		log.Printf("Invitation creation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create invitation",
		})
		return
	}

	// Das rohe Token wird nur bei der Erstellung zurückgegeben.
	// In der Datenbank wird ausschließlich der Hash gespeichert.
	c.JSON(http.StatusCreated, gin.H{
		"invitation":     invitation,
		"invitationCode": rawToken,
	})
}

// getAllInvitations ruft alle Einladungen des eigenen Tenants ab.
//
// @Summary Listet Einladungen des eigenen Unternehmens
// @Tags invitations
// @Produce json
// @Success 200 {array} database.Invitation
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/invitations [get]
func (app *application) getAllInvitations(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	invitations, err := app.models.Invitations.ListByTenant(
		c.Request.Context(),
		tenantID,
	)
	if err != nil {
		log.Printf("Invitation listing failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch invitations",
		})
		return
	}

	// Eine leere Liste als [] statt null zurückgeben.
	if invitations == nil {
		invitations = make([]*database.Invitation, 0)
	}

	c.JSON(http.StatusOK, invitations)
}

// revokeInvitation widerruft eine aktive Einladung des eigenen Tenants.
//
// @Summary Widerruft eine Einladung
// @Tags invitations
// @Produce json
// @Param id path int true "ID der Einladung"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/invitations/{id} [delete]
func (app *application) revokeInvitation(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	invitationID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid invitation ID",
		})
		return
	}

	err = app.models.Invitations.Revoke(
		c.Request.Context(),
		tenantID,
		invitationID,
	)
	if err != nil {
		if errors.Is(err, database.ErrInvalidInvitation) {
			// Auch bei einer anderen Tenant-ID wird nicht offengelegt,
			// ob eine fremde Einladung existiert.
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Invitation not found or no longer active",
			})
			return
		}

		log.Printf("Invitation revocation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to revoke invitation",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// deleteInvitation deletes an invitation permanently from the database.
//
// @Summary Löscht eine Einladung dauerhaft
// @Tags invitations
// @Produce json
// @Param id path int true "ID der Einladung"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/invitations/{id}/permanent [delete]
func (app *application) deleteInvitation(c *gin.Context) {
	_, tenantID, ok := eventRequestScope(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	invitationID, err := parsePositiveID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid invitation ID",
		})
		return
	}
	err = app.models.Invitations.Delete(
		c.Request.Context(),
		tenantID,
		invitationID,
	)
	if err != nil {
		if errors.Is(err, database.ErrInvalidInvitation) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Invitation not found",
			})
			return
		}
		log.Printf("Invitation deletion failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete invitation",
		})
		return
	}
	c.Status(http.StatusNoContent)
}

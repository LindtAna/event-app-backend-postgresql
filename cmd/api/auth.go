package main

import (
	"context"
	"errors"
	"event-app/internal/database"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
	tokenIssuer      = "event-app"

	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 7 * 24 * time.Hour
)

type registerRequest struct {
	RegistrationType string `json:"registrationType" binding:"required,oneof=owner member"`
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required,min=8"`
	Name             string `json:"name" binding:"required,min=1"`
	CompanyName      string `json:"companyName"`
	InvitationCode   string `json:"invitationCode"`
	Bio              string `json:"bio"`
	AvatarUrl        string `json:"avatarUrl"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type authResponse struct {
	AccessToken string        `json:"accessToken"`
	User        database.User `json:"user"`
}

type updateProfileRequest struct {
	Name      string `json:"name" binding:"required,min=2"`
	Bio       string `json:"bio"`
	AvatarUrl string `json:"avatarUrl"`
}

type authClaims struct {
	UserID    int    `json:"userId"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

// generateToken erstellt einen JWT mit explizitem Token-Typ.
func (app *application) generateToken(
	userID int,
	tokenType string,
	ttl time.Duration,
) (string, error) {
	if userID <= 0 {
		return "", errors.New("invalid user ID")
	}

	now := time.Now()

	claims := authClaims{
		UserID:    userID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(app.jwtSecret))
}

// erstellt einen kurzlebigen Access-Token(15 Min)
// generateAccessToken erstellt einen kurzlebigen Access-Token.
func (app *application) generateAccessToken(userID int) (string, error) {
	return app.generateToken(userID, accessTokenType, accessTokenTTL)
}

// erstellt einen langlebigeren Refresh-Token (7 Tage)
func (app *application) generateRefreshToken(userID int) (string, error) {
	return app.generateToken(userID, refreshTokenType, refreshTokenTTL)
}

// parseAuthToken überprüft Signatur, Algorithmus, Ablaufzeit, Issuer und Token-Typ
func (app *application) parseAuthToken(
	rawToken string,
	expectedType string,
) (*authClaims, error) {
	claims := &authClaims{}

	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			// Nur den tatsächlich verwendeten Algorithmus akzeptieren.
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}

			return []byte(app.jwtSecret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, err
	}

	if token == nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.UserID <= 0 || claims.TokenType != expectedType {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

// isActiveUserAndTenant prüft den aktuellen Status des Benutzers und seines Mandanten.
// TenantID und Rolle werden aus der Datenbank gelesen, nicht aus dem Request.
func (app *application) isActiveUserAndTenant(
	ctx context.Context,
	user *database.User,
) (bool, error) {
	if user == nil || user.TenantID <= 0 || user.Status != "active" {
		return false, nil
	}

	tenant, err := app.models.Tenants.Get(ctx, user.TenantID)
	if errors.Is(err, database.ErrTenantNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if tenant == nil || tenant.Status != "active" {
		return false, nil
	}

	return true, nil
}

// isSQLiteUniqueConstraintError erkennt UNIQUE- und PRIMARY-KEY-Verletzungen.
func isSQLiteUniqueConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE,
		sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return true
	default:
		return false
	}
}

// speichert den Refresh-Token in einem HttpOnly-Cookie.
func (app *application) setRefreshTokenCookie(
	c *gin.Context,
	refreshToken string,
) {
	app.writeRefreshTokenCookie(c, refreshToken, int(refreshTokenTTL.Seconds()))
}

// entfernt den Refresh-Token mit denselben Cookie-Attributen.
func (app *application) clearRefreshTokenCookie(c *gin.Context) {
	app.writeRefreshTokenCookie(c, "", -1)
}

func (app *application) writeRefreshTokenCookie(
	c *gin.Context,
	value string,
	maxAge int,
) {
	secure := c.Request.TLS != nil ||
		strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")

	origin := strings.TrimSpace(c.GetHeader("Origin"))

	if origin != "" && app.isAllowedOrigin(origin) &&
		strings.HasPrefix(strings.ToLower(origin), "https://") {
		secure = true
	}

	// Bei Requests ohne Origin die konfigurierte Frontend-URL berücksichtigen.
	if origin == "" &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(app.clientOrigin)), "https://") {
		secure = true
	}

	sameSite := http.SameSiteLaxMode
	if secure {
		// Erforderlich für Frontend und API auf unterschiedlichen Sites.
		sameSite = http.SameSiteNoneMode
	}

	c.SetSameSite(sameSite)
	c.SetCookie(
		"refreshToken",
		value,
		maxAge,
		"/api/v1/auth",
		"", // Host-only-Cookie: keine fest codierte localhost-Domain.
		secure,
		true, // HttpOnly
	)
}

// registerUser registriert entweder einen Owner samt Tenant oder einen eingeladenen Member.
//
// @Summary Registriert einen Owner oder ein eingeladenes Mitglied
// @Tags auth
// @Accept json
// @Produce json
// @Param user body registerRequest true "Registrierungsdaten"
// @Success 201 {object} database.User
// @Router /api/v1/auth/register [post]
func (app *application) registerUser(c *gin.Context) {
	var register registerRequest

	if err := c.ShouldBindJSON(&register); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid registration data",
		})
		return
	}

	if len(register.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Password must not exceed 72 bytes",
		})
		return
	}

	if strings.TrimSpace(register.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Name is required",
		})
		return
	}

	switch register.RegistrationType {
	case "owner":
		if strings.TrimSpace(register.CompanyName) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Company name is required for owner registration",
			})
			return
		}

	case "member":
		if strings.TrimSpace(register.InvitationCode) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invitation code is required for member registration",
			})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Unsupported registration type",
		})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(register.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		log.Printf("Password hashing failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not register user",
		})
		return
	}

	user := database.User{
		Email:     strings.TrimSpace(register.Email),
		Password:  string(hashedPassword),
		Name:      strings.TrimSpace(register.Name),
		Bio:       register.Bio,
		AvatarUrl: strings.TrimSpace(register.AvatarUrl),
	}

	ctx := c.Request.Context()

	switch register.RegistrationType {
	case "owner":
		// Erstellt Tenant und Owner gemeinsam in einer Transaktion.
		// Es werden ausdrücklich keine Kategorien angelegt.
		_, err = app.models.Tenants.CreateWithOwner(
			ctx,
			register.CompanyName,
			&user,
		)

	case "member":
		// TenantID und Rolle werden anhand der Einladung festgelegt.
		err = app.models.Invitations.RegisterMember(
			ctx,
			strings.TrimSpace(register.InvitationCode),
			&user,
		)
	}

	if err != nil {
		log.Printf(
			"Registration failed (type %s): %v",
			register.RegistrationType,
			err,
		)

		switch {
		case errors.Is(err, database.ErrInvalidTenant):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid company name",
			})

		case errors.Is(err, database.ErrTenantNameTaken):
			c.JSON(http.StatusConflict, gin.H{
				"error": "Company name is already in use",
			})

		case errors.Is(err, database.ErrInvalidInvitation):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invitation is invalid, expired, used or revoked",
			})

		case errors.Is(err, database.ErrInvalidUser):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid user data",
			})

		case isSQLiteUniqueConstraintError(err):
			c.JSON(http.StatusConflict, gin.H{
				"error": "Email address is already registered",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not register user",
			})
		}
		return
	}

	// User.Password имеет json:"-" и wird nicht an den Client ausgegeben.
	c.JSON(http.StatusCreated, user)
}

// updateProfile aktualisiert das Profil des angemeldeten Benutzers.
//
// @Summary Aktualisiert das Profil des angemeldeten Benutzers
// @Tags users
// @Accept json
// @Produce json
// @Param profile body updateProfileRequest true "Profildaten"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/users/profile [put]
// @Security BearerAuth
func (app *application) updateProfile(c *gin.Context) {
	userValue, exists := c.Get("user")
	authUser, ok := userValue.(*database.User)

	if !exists ||
		!ok ||
		authUser == nil ||
		authUser.Id <= 0 ||
		authUser.TenantID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid profile data",
		})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Name is required",
		})
		return
	}

	ctx := c.Request.Context()

	err := app.models.Users.UpdateProfile(
		ctx,
		authUser.TenantID,
		authUser.Id,
		name,
		req.Bio,
		strings.TrimSpace(req.AvatarUrl),
	)
	if err != nil {
		if errors.Is(err, database.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found",
			})
			return
		}
		if errors.Is(err, database.ErrInvalidUser) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid user data",
			})
			return
		}
		log.Printf("Profile update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update profile",
		})
		return
	}

	updatedUser, err := app.models.Users.GetByIDInTenant(
		ctx,
		authUser.TenantID,
		authUser.Id,
	)
	if err != nil {
		log.Printf("Updated user lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch updated profile",
		})
		return
	}
	if updatedUser == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user":    updatedUser,
	})
}

// login meldet einen aktiven Benutzer an.
//
// @Summary Meldet einen Benutzer an
// @Tags auth
// @Accept json
// @Produce json
// @Param user body loginRequest true "Login-Daten"
// @Success 200 {object} authResponse
// @Router /api/v1/auth/login [post]
func (app *application) login(c *gin.Context) {
	var auth loginRequest

	if err := c.ShouldBindJSON(&auth); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid login data",
		})
		return
	}

	if len(auth.Password) > 72 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	existingUser, err := app.models.Users.GetByEmail(
		c.Request.Context(),
		strings.TrimSpace(auth.Email),
	)
	if err != nil {
		log.Printf("User lookup during login failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Something went wrong",
		})
		return
	}

	if existingUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(existingUser.Password),
		[]byte(auth.Password),
	); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	active, err := app.isActiveUserAndTenant(
		c.Request.Context(),
		existingUser,
	)
	if err != nil {
		log.Printf("Account status check during login failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Something went wrong",
		})
		return
	}

	if !active {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Account is unavailable",
		})
		return
	}

	accessToken, err := app.generateAccessToken(existingUser.Id)
	if err != nil {
		log.Printf("Access token generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not create session",
		})
		return
	}

	refreshToken, err := app.generateRefreshToken(existingUser.Id)
	if err != nil {
		log.Printf("Refresh token generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not create session",
		})
		return
	}

	app.setRefreshTokenCookie(c, refreshToken)

	c.JSON(http.StatusOK, authResponse{
		AccessToken: accessToken,
		User:        *existingUser,
	})
}

// refreshToken erstellt einen neuen Access-Token anhand des Refresh-Cookies.
//
// @Summary Erneuert den Access-Token
// @Tags auth
// @Produce json
// @Success 200 {object} authResponse
// @Router /api/v1/auth/refresh [post]
func (app *application) refreshToken(c *gin.Context) {
	cookie, err := c.Cookie("refreshToken")
	if err != nil || strings.TrimSpace(cookie) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Refresh token missing",
		})
		return
	}

	claims, err := app.parseAuthToken(cookie, refreshTokenType)
	if err != nil {
		app.clearRefreshTokenCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid refresh token",
		})
		return
	}

	user, err := app.models.Users.Get(
		c.Request.Context(),
		claims.UserID)
	if err != nil {
		log.Printf("User lookup during token refresh failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not refresh session",
		})
		return
	}

	if user == nil {
		app.clearRefreshTokenCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Session is no longer valid",
		})
		return
	}

	active, err := app.isActiveUserAndTenant(c.Request.Context(), user)
	if err != nil {
		log.Printf("Account status check during token refresh failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not refresh session",
		})
		return
	}

	if !active {
		app.clearRefreshTokenCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Account is unavailable",
		})
		return
	}

	newAccessToken, err := app.generateAccessToken(user.Id)
	if err != nil {
		log.Printf("Access token generation during refresh failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not refresh session",
		})
		return
	}

	c.JSON(http.StatusOK, authResponse{
		AccessToken: newAccessToken,
		User:        *user,
	})
}

// logout entfernt das Refresh-Cookie aus dem Browser.
//
// @Summary Meldet den Benutzer ab
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/auth/logout [post]
func (app *application) logout(c *gin.Context) {
	app.clearRefreshTokenCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully logged out",
	})
}

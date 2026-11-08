package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ibnu-hafidz/web-v2/internal/models"
	"github.com/ibnu-hafidz/web-v2/internal/utils"
	"gorm.io/gorm"
)

var errRefreshInvalid = errors.New("invalid refresh session")

func tokenDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func newRefreshToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (h *AuthHandler) refreshCookie(c *fiber.Ctx, value string, expires time.Time) {
	c.Cookie(&fiber.Cookie{Name: "refresh_token", Value: value, Expires: expires,
		HTTPOnly: true, Secure: h.cfg.Environment == "production", SameSite: "Lax", Path: "/api/auth"})
}

func (h *AuthHandler) createRefreshSession(user models.User) (string, time.Time, error) {
	token, err := newRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(7 * 24 * time.Hour)
	digest := tokenDigest(token)
	row := models.RefreshSession{TokenHash: digest, FamilyID: digest, UserID: user.ID,
		CredentialHash: tokenDigest(user.Password), ExpiresAt: expires}
	if err := h.db.Create(&row).Error; err != nil {
		return "", time.Time{}, err
	}
	// Keep replay records only as long as their original session can be used.
	h.db.Where("expires_at < ?", time.Now()).Delete(&models.RefreshSession{})
	return token, expires, nil
}

func revokeRefreshSessions(db *gorm.DB, userID uint) error {
	return db.Model(&models.RefreshSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now()).Error
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	raw := c.Cookies("refresh_token")
	if len(raw) != 43 {
		return h.rejectRefresh(c)
	}
	var old models.RefreshSession
	if err := h.db.First(&old, "token_hash = ?", tokenDigest(raw)).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(503, "Refresh temporarily unavailable")
		}
		return h.rejectRefresh(c)
	}
	now := time.Now()
	if old.RevokedAt != nil || !old.ExpiresAt.After(now) {
		return h.rejectRefresh(c)
	}
	if old.ConsumedAt != nil {
		if err := h.revokeRefreshFamily(old.FamilyID); err != nil {
			return fiber.NewError(503, "Refresh temporarily unavailable")
		}
		return h.rejectRefresh(c)
	}
	var user models.User
	if err := h.db.Preload("Roles.Permissions").First(&user, old.UserID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(503, "Refresh temporarily unavailable")
		}
		return h.rejectRefresh(c)
	}
	if old.CredentialHash != tokenDigest(user.Password) {
		if err := h.revokeRefreshFamily(old.FamilyID); err != nil {
			return fiber.NewError(503, "Refresh temporarily unavailable")
		}
		return h.rejectRefresh(c)
	}
	next, err := newRefreshToken()
	if err != nil {
		return fiber.NewError(500, "Failed to refresh session")
	}
	access, err := utils.GenerateAccessToken(user.ID, user.Email, h.cfg.JWTSecret)
	if err != nil {
		return fiber.NewError(500, "Failed to generate token")
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.RefreshSession{}).
			Where("token_hash = ? AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > ?", old.TokenHash, now).
			Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errRefreshInvalid
		}
		return tx.Create(&models.RefreshSession{TokenHash: tokenDigest(next), FamilyID: old.FamilyID,
			UserID: user.ID, CredentialHash: old.CredentialHash, ExpiresAt: old.ExpiresAt}).Error
	})
	if errors.Is(err, errRefreshInvalid) {
		if err := h.revokeRefreshFamily(old.FamilyID); err != nil {
			return fiber.NewError(503, "Refresh temporarily unavailable")
		}
		return h.rejectRefresh(c)
	}
	if err != nil {
		return fiber.NewError(503, "Refresh temporarily unavailable")
	}
	c.Set("Cache-Control", "no-store")
	h.refreshCookie(c, next, old.ExpiresAt)
	return c.JSON(models.AuthResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: 900, User: user})
}

func (h *AuthHandler) revokeRefreshFamily(familyID string) error {
	return h.db.Model(&models.RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", time.Now()).Error
}

func (h *AuthHandler) rejectRefresh(c *fiber.Ctx) error {
	h.refreshCookie(c, "", time.Now().Add(-time.Hour))
	return c.Status(401).JSON(models.ErrorResponse{Error: "unauthorized", Message: "Session expired; please sign in again"})
}

// Logout is cookie-authenticated so it works after an access token expires.
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	if raw := c.Cookies("refresh_token"); len(raw) == 43 {
		var session models.RefreshSession
		err := h.db.First(&session, "token_hash = ?", tokenDigest(raw)).Error
		if err == nil {
			if err := h.revokeRefreshFamily(session.FamilyID); err != nil {
				return fiber.NewError(503, "Logout temporarily unavailable")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(503, "Logout temporarily unavailable")
		}
	}
	c.Set("Cache-Control", "no-store")
	h.refreshCookie(c, "", time.Now().Add(-time.Hour))
	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

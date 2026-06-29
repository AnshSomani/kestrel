package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type APIKeyResponse struct {
	ID        string    `json:"id"`
	Key       string    `json:"key,omitempty"` // Only return full key on creation
	Prefix    string    `json:"prefix"`        // sk_live_...
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) ListAPIKeys(c echo.Context) error {
	ctx := c.Request().Context()
	claims, ok := c.Get("user").(*JWTClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "invalid user ID"})
	}

	rows, err := h.pool.Query(ctx, `
		SELECT id, key, created_at 
		FROM api_keys 
		WHERE user_id = $1 
		ORDER BY created_at DESC
	`, tenantID)
	
	if err != nil {
		h.logger.Error("failed to query api keys", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal error"})
	}
	defer rows.Close()

	var keys []APIKeyResponse
	for rows.Next() {
		var k APIKeyResponse
		var fullKey string
		if err := rows.Scan(&k.ID, &fullKey, &k.CreatedAt); err != nil {
			continue
		}
		
		// Only send prefix for security (e.g. sk_live_a8d92...)
		k.Prefix = fullKey[:12] + "..."
		keys = append(keys, k)
	}

	if keys == nil {
		keys = []APIKeyResponse{}
	}

	return c.JSON(http.StatusOK, echo.Map{"keys": keys})
}

func (h *Handler) CreateAPIKey(c echo.Context) error {
	ctx := c.Request().Context()
	claims, ok := c.Get("user").(*JWTClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "invalid user ID"})
	}

	// Generate a secure random key
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to generate key"})
	}
	newKey := "sk_live_" + hex.EncodeToString(b)

	var k APIKeyResponse
	err = h.pool.QueryRow(ctx, `
		INSERT INTO api_keys (key, user_id) 
		VALUES ($1, $2) 
		RETURNING id, key, created_at
	`, newKey, tenantID).Scan(&k.ID, &k.Key, &k.CreatedAt)

	if err != nil {
		h.logger.Error("failed to create api key", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to create key"})
	}
	
	k.Prefix = k.Key[:12] + "..."
	return c.JSON(http.StatusCreated, k)
}

func (h *Handler) DeleteAPIKey(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	keyID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid key ID"})
	}

	claims, ok := c.Get("user").(*JWTClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
	}
	tenantID, _ := uuid.Parse(claims.UserID)

	res, err := h.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, keyID, tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal error"})
	}
	if res.RowsAffected() == 0 {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "key not found"})
	}

	return c.NoContent(http.StatusNoContent)
}

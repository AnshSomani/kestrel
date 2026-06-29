package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) ListUsers(c echo.Context) error {
	ctx := c.Request().Context()
	
	rows, err := h.pool.Query(ctx, "SELECT id, email, role, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		h.logger.Error("failed to query users", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to list users"})
	}
	defer rows.Close()

	var users []UserResponse
	for rows.Next() {
		var u UserResponse
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	return c.JSON(http.StatusOK, echo.Map{
		"users": users,
	})
}

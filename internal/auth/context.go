package auth

import (
	"context"

	"github.com/Stocist/discard/internal/models"
)

type contextKey int

const userContextKey contextKey = iota

// ContextWithUser returns a new context carrying the given user.
func ContextWithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// UserFromContext extracts the authenticated user from the context.
// Returns nil if no user is present.
func UserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(userContextKey).(*models.User)
	return u
}

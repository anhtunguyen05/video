package auth

import (
	"context"
	"fmt"
)

type Principal struct {
	ID string
}

type CurrentUserProvider interface {
	CurrentUser(context.Context) (Principal, error)
}

// StaticPrincipal is the development identity used until real session auth is implemented.
type StaticPrincipal struct {
	ID string
}

func (provider StaticPrincipal) CurrentUser(_ context.Context) (Principal, error) {
	if provider.ID == "" {
		return Principal{}, fmt.Errorf("development principal is not configured")
	}
	return Principal{ID: provider.ID}, nil
}

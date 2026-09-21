// Package authz scopes queries to repositories the user may access.
package authz

import (
	"context"

	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

type Service struct {
	store *store.Store
}

func New(st *store.Store) *Service { return &Service{store: st} }

// Scope describes how list queries should filter.
type Scope struct {
	UserID       int64
	BootstrapAll bool
}

func (s *Service) ScopeFor(user *models.User) Scope {
	if user == nil {
		return Scope{}
	}
	if user.IsBootstrapAdmin {
		return Scope{UserID: user.ID, BootstrapAll: true}
	}
	return Scope{UserID: user.ID, BootstrapAll: false}
}

func (s *Service) CanAccessRepo(ctx context.Context, user *models.User, repoID int64) (bool, error) {
	if user == nil {
		return false, nil
	}
	if user.IsBootstrapAdmin {
		return true, nil
	}
	return s.store.UserCanAccessRepo(ctx, user.ID, repoID)
}

func (s *Service) RefreshFromRepoList(ctx context.Context, userID int64, repos []models.Repository) error {
	ids := make([]int64, 0, len(repos))
	for _, r := range repos {
		if r.ID > 0 {
			ids = append(ids, r.ID)
		}
	}
	return s.store.ReplaceUserRepoAccess(ctx, userID, ids)
}

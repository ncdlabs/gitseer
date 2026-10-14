// Package authz scopes queries to repositories the user may access.
package authz

import (
	"context"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
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
	repo, err := s.store.GetRepositoryByID(ctx, repoID)
	if err != nil || repo == nil || repo.DeletedAt != nil {
		return false, nil
	}
	if user.IsBootstrapAdmin {
		return true, nil
	}
	return s.store.UserCanAccessRepo(ctx, user.ID, repoID)
}

// CanAccessRepos batch-checks access for multiple repos, returning the subset the user may access.
// This avoids the N+1 query pattern of calling CanAccessRepo in a loop.
func (s *Service) CanAccessRepos(ctx context.Context, user *models.User, repoIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(repoIDs))
	if user == nil || len(repoIDs) == 0 {
		return out, nil
	}
	if user.IsBootstrapAdmin {
		// Bootstrap admin can access all non-deleted repos; still need to check existence/deleted.
		for _, id := range repoIDs {
			repo, err := s.store.GetRepositoryByID(ctx, id)
			if err == nil && repo != nil && repo.DeletedAt == nil {
				out[id] = true
			}
		}
		return out, nil
	}
	// Non-admin: fetch all repos in one query, then check access in batch.
	repos, err := s.store.GetRepositoriesByIDs(ctx, repoIDs)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return out, nil
	}
	accessible, err := s.store.UserCanAccessRepos(ctx, user.ID, repoIDs)
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		if repo.DeletedAt != nil {
			continue
		}
		if accessible[repo.ID] {
			out[repo.ID] = true
		}
	}
	return out, nil
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

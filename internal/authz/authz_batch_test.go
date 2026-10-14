package authz

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestCanAccessRepos_Batch(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "az_batch.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	az := New(st)

	inst, _ := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	r1, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "a", Name: "one", FullName: "a/one"})
	r2, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 2, Owner: "b", Name: "two", FullName: "b/two"})
	r3, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 3, Owner: "c", Name: "three", FullName: "c/three"})

	u1, _ := st.UpsertGiteaUser(ctx, &inst.ID, models.User{Login: "alice", GiteaUserID: int64Ptr(10)}, nil)
	u2, _ := st.UpsertGiteaUser(ctx, &inst.ID, models.User{Login: "bob", GiteaUserID: int64Ptr(11)}, nil)

	// alice gets access to r1 and r3; bob gets access to r2.
	_ = az.RefreshFromRepoList(ctx, u1.ID, []models.Repository{*r1, *r3})
	_ = az.RefreshFromRepoList(ctx, u2.ID, []models.Repository{*r2})

	// alice batch check
	access, err := az.CanAccessRepos(ctx, u1, []int64{r1.ID, r2.ID, r3.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !access[r1.ID] || access[r2.ID] || !access[r3.ID] {
		t.Fatalf("alice access map wrong: %+v", access)
	}

	// bob batch check
	access, err = az.CanAccessRepos(ctx, u2, []int64{r1.ID, r2.ID, r3.ID})
	if err != nil {
		t.Fatal(err)
	}
	if access[r1.ID] || !access[r2.ID] || access[r3.ID] {
		t.Fatalf("bob access map wrong: %+v", access)
	}
}

func TestCanAccessRepos_BootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "az_batch_boot.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	az := New(st)

	inst, _ := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	r1, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "a", Name: "one", FullName: "a/one"})
	r2, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 2, Owner: "b", Name: "two", FullName: "b/two"})

	boot, _ := st.EnsureBootstrapUser(ctx)
	_ = az.RefreshFromRepoList(ctx, boot.ID, []models.Repository{*r1})

	// Bootstrap admin can access any non-deleted repo (matches CanAccessRepo behavior).
	access, err := az.CanAccessRepos(ctx, boot, []int64{r1.ID, r2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !access[r1.ID] || !access[r2.ID] {
		t.Fatalf("bootstrap access map wrong: %+v", access)
	}
}

func TestCanAccessRepos_ExcludesDeleted(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "az_batch_del.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	az := New(st)

	inst, _ := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	r1, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "a", Name: "one", FullName: "a/one"})

	u1, _ := st.UpsertGiteaUser(ctx, &inst.ID, models.User{Login: "alice", GiteaUserID: int64Ptr(10)}, nil)
	_ = az.RefreshFromRepoList(ctx, u1.ID, []models.Repository{*r1})

	// Soft-delete r1 — access should be gone.
	_ = st.SoftDeleteRepository(ctx, r1.ID)
	access, err := az.CanAccessRepos(ctx, u1, []int64{r1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if access[r1.ID] {
		t.Fatal("soft-deleted repo should not be accessible")
	}
}

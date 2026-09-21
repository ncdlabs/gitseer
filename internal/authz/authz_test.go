package authz

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

func TestAuthzIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "az.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st := store.New(db)
	az := New(st)
	inst, _ := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	r1, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "a", Name: "one", FullName: "a/one"})
	r2, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 2, Owner: "b", Name: "two", FullName: "b/two"})

	u1, _ := st.UpsertGiteaUser(ctx, &inst.ID, models.User{Login: "alice", GiteaUserID: int64Ptr(10)})
	u2, _ := st.UpsertGiteaUser(ctx, &inst.ID, models.User{Login: "bob", GiteaUserID: int64Ptr(11)})
	_ = az.RefreshFromRepoList(ctx, u1.ID, []models.Repository{*r1})
	_ = az.RefreshFromRepoList(ctx, u2.ID, []models.Repository{*r2})

	list1, total1, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{UserID: u1.ID})
	if err != nil || total1 != 1 || list1[0].FullName != "a/one" {
		t.Fatalf("alice saw %+v total=%d", list1, total1)
	}
	list2, total2, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{UserID: u2.ID})
	if err != nil || total2 != 1 || list2[0].FullName != "b/two" {
		t.Fatalf("bob saw %+v total=%d", list2, total2)
	}
	ok, _ := az.CanAccessRepo(ctx, u1, r2.ID)
	if ok {
		t.Fatal("alice must not access bob repo")
	}
}

func int64Ptr(v int64) *int64 { return &v }

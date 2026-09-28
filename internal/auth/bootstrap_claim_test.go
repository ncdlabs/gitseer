package auth_test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/store"
)

func newAuthService(t *testing.T, cfg auth.Config) (*auth.Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.NewWithDriver(db, "sqlite")
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = time.Hour
	}
	cfg.CookiePath = "/"
	return auth.New(st, cfg), st
}

func TestClaimThenLogin(t *testing.T) {
	svc, _ := newAuthService(t, auth.Config{})
	ctx := context.Background()

	info, err := svc.BootstrapInfo(ctx, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Unclaimed {
		t.Fatal("expected unclaimed")
	}

	user, token, err := svc.ClaimBootstrap(ctx, "opsadmin", "password123", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if user.Login != "opsadmin" || !user.IsBootstrapAdmin || token == "" {
		t.Fatalf("unexpected claim result: %+v token=%q", user, token)
	}

	if _, _, err := svc.ClaimBootstrap(ctx, "other", "password123", "", ""); err == nil {
		t.Fatal("second claim should fail")
	}

	info, err = svc.BootstrapInfo(ctx, false, false)
	if err != nil || info.Unclaimed || !info.LoginEnabled {
		t.Fatalf("after claim: %+v err=%v", info, err)
	}

	u2, _, err := svc.LoginBootstrap(ctx, "opsadmin", "password123", "", "")
	if err != nil || u2.Login != "opsadmin" {
		t.Fatalf("login: user=%+v err=%v", u2, err)
	}
	if _, _, err := svc.LoginBootstrap(ctx, "opsadmin", "wrong-password", "", ""); err == nil {
		t.Fatal("bad password should fail")
	}
}

func TestElevateAndPasswordReset(t *testing.T) {
	svc, _ := newAuthService(t, auth.Config{})
	ctx := context.Background()
	_, token, err := svc.ClaimBootstrap(ctx, "admin", "password123", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}

	req := httptestReq(token)
	user, sess, err := svc.UserFromRequest(ctx, req)
	if err != nil || user == nil || sess == nil {
		t.Fatalf("session: %v", err)
	}

	until, err := svc.ElevateBootstrap(ctx, sess, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if until.Before(time.Now().UTC()) {
		t.Fatal("until should be in the future")
	}
	if !auth.EffectiveBootstrapAdmin(user, sess) {
		t.Fatal("expected elevated admin")
	}

	if err := svc.SetBootstrapPassword(ctx, "password123", "newpassword99"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LoginBootstrap(ctx, "admin", "newpassword99", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.LoginBootstrap(ctx, "admin", "password123", "", ""); err == nil {
		t.Fatal("old password should fail")
	}
}

func TestKeepAfterSetupDisablesLogin(t *testing.T) {
	keep := false
	svc, st := newAuthService(t, auth.Config{BootstrapKeepAfterSetup: &keep})
	ctx := context.Background()
	if _, _, err := svc.ClaimBootstrap(ctx, "admin", "password123", "", ""); err != nil {
		t.Fatal(err)
	}
	_ = st.SetBootstrapKeepAfterSetup(ctx, false)

	info, err := svc.BootstrapInfo(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.LoginEnabled {
		t.Fatal("login should be disabled after setup when keep=false")
	}
	info2, err := svc.BootstrapInfo(ctx, true, true)
	if err != nil || !info2.LoginEnabled {
		t.Fatal("allow_skip_setup should re-enable login")
	}
	if !info.HasPassword {
		t.Fatal("password should still exist for elevate")
	}
}

func httptestReq(token string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	return req
}

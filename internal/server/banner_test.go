package server

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/settings"
)

func TestPrintStartupBannerFirstRun(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Listen = "0.0.0.0:8090"
	cfg.Server.ExternalURL = "https://gitseer.example.com/gitseer"
	cfg.Auth.BootstrapUsername = "admin"
	cfg.UI.InstanceName = "Lab"

	setup := settings.SetupStatus{
		NeedsSetup:     true,
		FirstRun:       true,
		SetupCompleted: false,
		Reasons:        []string{"setup wizard not finished"},
	}

	var buf bytes.Buffer
	PrintStartupBanner(&buf, cfg, "1.2.3", setup)
	out := buf.String()

	for _, want := range []string{
		"by ncdLabs",
		"GitSeer started",
		"v1.2.3",
		"Instance: Lab",
		"Services",
		"0.0.0.0:8090",
		"https://gitseer.example.com/gitseer/health/live",
		"First Run — Setup Wizard",
		"Claim Bootstrap",
		`username "admin"`,
		"Missing       setup wizard not finished",
		"https://gitseer.example.com/gitseer/",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q\n%s", want, out)
		}
	}
}

func TestPrintStartupBannerMisconfigured(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Listen = "127.0.0.1:8090"
	setup := settings.SetupStatus{
		NeedsSetup:           true,
		FirstRun:             false,
		SetupCompleted:       true,
		EncryptionConfigured: false,
		Reasons:              []string{"encryption key not configured (GITSEER_ENCRYPTION_KEY, encryption_key_file, or wizard file)"},
	}
	var buf bytes.Buffer
	PrintStartupBanner(&buf, cfg, "1.0.0", setup)
	out := buf.String()
	if !strings.Contains(out, "Setup Required — Incomplete Configuration") {
		t.Fatalf("expected misconfigured heading\n%s", out)
	}
	if !strings.Contains(out, "Missing       encryption key") {
		t.Fatalf("expected missing reason\n%s", out)
	}
}

func TestPrintStartupBannerConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Listen = "0.0.0.0:8090"
	cfg.Server.ExternalURL = "https://gitseer.example.com/gitseer"

	var buf bytes.Buffer
	PrintStartupBanner(&buf, cfg, "1.2.3", settings.SetupStatus{NeedsSetup: false, SetupCompleted: true})
	out := buf.String()

	for _, want := range []string{
		"Connect",
		"Public URL    https://gitseer.example.com/gitseer",
		"/api/webhooks/{forge}/{instanceID}",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "First Run") || strings.Contains(out, "Setup Required") {
		t.Fatalf("configured banner should not show setup section\n%s", out)
	}
}

func TestPrintStartupBannerLocalFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Listen = "127.0.0.1:9000"
	cfg.Server.ExternalURL = ""

	var buf bytes.Buffer
	PrintStartupBanner(&buf, cfg, "9.9.9", settings.SetupStatus{NeedsSetup: true, FirstRun: true})
	out := buf.String()

	if !strings.Contains(out, "http://127.0.0.1:9000/") {
		t.Fatalf("expected local browse URL\n%s", out)
	}
	if !strings.Contains(out, "First Run — Setup Wizard") {
		t.Fatalf("expected first-run wizard section\n%s", out)
	}
}

func TestMaybeOpenSetupWizardSkippedWhenConfigured(t *testing.T) {
	cfg := config.Default()
	var buf bytes.Buffer
	if MaybeOpenSetupWizard(&buf, cfg, settings.SetupStatus{NeedsSetup: false}) {
		t.Fatal("expected no browser open when setup not needed")
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}

func TestMaybeOpenSetupWizardSkippedWhenNoBrowser(t *testing.T) {
	t.Setenv("GITSEER_NO_BROWSER", "1")
	cfg := config.Default()
	var buf bytes.Buffer
	if MaybeOpenSetupWizard(&buf, cfg, settings.SetupStatus{NeedsSetup: true, FirstRun: true}) {
		t.Fatal("expected skip when GITSEER_NO_BROWSER=1")
	}
}

func TestShouldAutoOpenBrowserRespectsEnv(t *testing.T) {
	t.Setenv("GITSEER_NO_BROWSER", "1")
	t.Setenv("CI", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("container", "")
	if shouldAutoOpenBrowser() {
		t.Fatal("expected false with GITSEER_NO_BROWSER=1")
	}
}

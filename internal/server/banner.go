package server

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/settings"
)

// asciiLogo is the GitSeer wordmark shown on binary startup (by ncdLabs).
const asciiLogo = `
     .-o-.     ____ _ _   ____
    /  *  \   / ___(_) |_| ___|  ___  ___ _ __
   (  /|\  ) | |  _| | __|___ \ / _ \/ _ \ '__|
    \  |  /  | |_| | | |_ ___) |  __/  __/ |
     ` + "`" + `o-o'    \____|_|\__|____/ \___|\___|_|
                         by ncdLabs
`

// PrintStartupBanner writes the process-started screen to w (stdout when nil).
// When setup.NeedsSetup (first run, incomplete config, or missing forge/encryption),
// Connect points at the Setup Wizard; callers should invoke MaybeOpenSetupWizard after listen.
func PrintStartupBanner(w io.Writer, cfg config.Config, version string, setup settings.SetupStatus) {
	if w == nil {
		w = os.Stdout
	}
	listen := strings.TrimSpace(cfg.Server.Listen)
	if listen == "" {
		listen = "0.0.0.0:8090"
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		port = "8090"
	}
	prefix := cfg.PathPrefix()
	base := publicBaseURL(cfg, port, prefix)

	fmt.Fprint(w, asciiLogo)
	fmt.Fprintf(w, "\n  GitSeer started  ·  v%s\n", version)
	if name := strings.TrimSpace(cfg.UI.InstanceName); name != "" && !strings.EqualFold(name, "GitSeer") {
		fmt.Fprintf(w, "  Instance: %s\n", name)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Services")
	fmt.Fprintf(w, "    %-18s %s\n", "HTTP (UI + API)", listen)
	fmt.Fprintf(w, "    %-18s %s\n", "Health", joinURL(base, "/health/live"))
	fmt.Fprintf(w, "    %-18s %s\n", "Metrics", joinURL(base, "/metrics"))
	fmt.Fprintf(w, "    %-18s %s\n", "Webhooks", joinURL(base, "/api/webhooks/{forge}/{id}"))
	fmt.Fprintf(w, "    %-18s %s\n", "Events (SSE)", joinURL(base, "/api/v1/events"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Workers")
	fmt.Fprintln(w, "    sync reconcile · webhook processor · retention · attention sweep")
	fmt.Fprintln(w, "    notification outbox · ACL refresh · realtime hub")
	fmt.Fprintln(w)
	if setup.NeedsSetup {
		if setup.FirstRun {
			fmt.Fprintln(w, "  First Run — Setup Wizard")
		} else {
			fmt.Fprintln(w, "  Setup Required — Incomplete Configuration")
		}
		fmt.Fprintf(w, "    Open          %s\n", joinURL(base, "/"))
		fmt.Fprintln(w, "    Steps         Claim Bootstrap → Prepare → Choose Forge → Connect → Validate → Finish")
		if u := strings.TrimSpace(cfg.Auth.BootstrapUsername); u != "" {
			fmt.Fprintf(w, "    Suggested     username %q\n", u)
		}
		for _, reason := range setup.Reasons {
			fmt.Fprintf(w, "    Missing       %s\n", reason)
		}
		fmt.Fprintln(w, "    Tip           set server.external_url before OAuth callbacks and forge webhooks")
	} else {
		fmt.Fprintln(w, "  Connect")
		fmt.Fprintf(w, "    Open          %s\n", joinURL(base, "/"))
		fmt.Fprintf(w, "    Webhooks      POST %s\n", joinURL(base, "/api/webhooks/{forge}/{instanceID}"))
		if ext := strings.TrimSpace(cfg.Server.ExternalURL); ext != "" {
			fmt.Fprintf(w, "    Public URL    %s\n", strings.TrimRight(ext, "/"))
		}
	}
	fmt.Fprintln(w)
}

// StartupBrowseURL returns the URL operators should open (UI root, including path prefix).
func StartupBrowseURL(cfg config.Config) string {
	listen := strings.TrimSpace(cfg.Server.Listen)
	if listen == "" {
		listen = "0.0.0.0:8090"
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		port = "8090"
	}
	return joinURL(publicBaseURL(cfg, port, cfg.PathPrefix()), "/")
}

// MaybeOpenSetupWizard opens the UI in a local browser when setup is needed and it is
// safe to do so (interactive TTY, not CI/container). Returns true when a launch
// was attempted. Set GITSEER_NO_BROWSER=1 to skip.
func MaybeOpenSetupWizard(w io.Writer, cfg config.Config, setup settings.SetupStatus) bool {
	if !setup.NeedsSetup || !shouldAutoOpenBrowser() {
		return false
	}
	if w == nil {
		w = os.Stdout
	}
	url := StartupBrowseURL(cfg)
	fmt.Fprintf(w, "  Opening Setup Wizard…\n    %s\n\n", url)
	// Brief delay so ListenAndServe is accepting connections before the browser hits it.
	time.Sleep(400 * time.Millisecond)
	_ = openBrowser(url)
	return true
}

func shouldAutoOpenBrowser() bool {
	if os.Getenv("GITSEER_NO_BROWSER") == "1" || os.Getenv("CI") != "" {
		return false
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return false
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return false
	}
	// Podman / OCI often set this; skip headless container starts.
	if os.Getenv("container") != "" {
		return false
	}
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func publicBaseURL(cfg config.Config, port, prefix string) string {
	if ext := strings.TrimSpace(cfg.Server.ExternalURL); ext != "" {
		return strings.TrimRight(ext, "/")
	}
	host := "127.0.0.1"
	if h, _, err := net.SplitHostPort(strings.TrimSpace(cfg.Server.Listen)); err == nil {
		switch h {
		case "", "0.0.0.0", "::", "[::]":
			// keep loopback for local browse URL
		default:
			host = h
		}
	}
	base := fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
	if prefix != "" {
		base += prefix
	}
	return base
}

func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if path == "" || path == "/" {
		return base + "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Avoid double path when base already ends with the prefix segment.
	u, err := url.Parse(base)
	if err != nil {
		return base + path
	}
	return strings.TrimRight(u.String(), "/") + path
}

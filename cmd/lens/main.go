package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ncdlabs/gitea-lens/internal/config"
	applog "github.com/ncdlabs/gitea-lens/internal/log"
	"github.com/ncdlabs/gitea-lens/internal/server"
	"github.com/ncdlabs/gitea-lens/internal/uiinstall"
)

var version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "serve")
	}
	switch os.Args[1] {
	case "serve":
		serveCmd(os.Args[2:])
	case "version":
		fmt.Println(version)
	case "install-ui":
		uiinstall.InstallCmd(os.Args[2:])
	case "uninstall-ui":
		uiinstall.UninstallCmd(os.Args[2:])
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `Gitea Lens — CI/CD and PR operations console for Gitea

Usage:
  lens serve [flags]
  lens version
  lens install-ui --custom-path DIR --lens-url URL
  lens uninstall-ui --custom-path DIR

`)
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", envOr("LENS_CONFIG", ""), "path to config.yaml")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	logger := applog.Setup(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(cfg, logger, version)
	if err != nil {
		logger.Error("server init failed", "err", err)
		os.Exit(1)
	}
	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

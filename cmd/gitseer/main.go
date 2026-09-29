package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/database"
	_ "github.com/ncdlabs/gitseer/internal/forge/all"
	applog "github.com/ncdlabs/gitseer/internal/log"
	"github.com/ncdlabs/gitseer/internal/backup"
	"github.com/ncdlabs/gitseer/internal/server"
	"github.com/ncdlabs/gitseer/internal/settings"
	"github.com/ncdlabs/gitseer/internal/store"
	"github.com/ncdlabs/gitseer/internal/uiinstall"
)

var version = "1.0.8"

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
	case "backup":
		backupCmd(os.Args[2:])
	case "restore":
		restoreCmd(os.Args[2:])
	case "rotate-encryption-key":
		rotateEncryptionKeyCmd(os.Args[2:])
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `GitSeer — CI/CD and PR operations console for Gitea and GitHub

Usage:
  gitseer serve [flags]
  gitseer version
  gitseer backup --out DIR [--config path]
  gitseer restore --from DIR [--config path] [--force]
  gitseer rotate-encryption-key [--config path] (--new-key STR | --new-key-file PATH | --generate)
  gitseer install-ui --custom-path DIR --gitseer-url URL
  gitseer uninstall-ui --custom-path DIR

`)
}

func rotateEncryptionKeyCmd(args []string) {
	fs := flag.NewFlagSet("rotate-encryption-key", flag.ExitOnError)
	cfgPath := fs.String("config", envOr("GITSEER_CONFIG", ""), "path to config.yaml")
	newKey := fs.String("new-key", "", "new encryption passphrase (≥24 chars)")
	newKeyFile := fs.String("new-key-file", "", "read new passphrase from file")
	generate := fs.Bool("generate", false, "generate a new high-entropy passphrase")
	_ = fs.Parse(args)

	sources := 0
	if strings.TrimSpace(*newKey) != "" {
		sources++
	}
	if strings.TrimSpace(*newKeyFile) != "" {
		sources++
	}
	if *generate {
		sources++
	}
	if sources != 1 {
		fmt.Fprintf(os.Stderr, "rotate-encryption-key: provide exactly one of --new-key, --new-key-file, or --generate\n")
		os.Exit(2)
	}

	passphrase := strings.TrimSpace(*newKey)
	if *newKeyFile != "" {
		raw, err := os.ReadFile(*newKeyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rotate-encryption-key: read --new-key-file: %v\n", err)
			os.Exit(1)
		}
		passphrase = strings.TrimSpace(string(raw))
	}
	if *generate {
		var err error
		passphrase, err = settings.RandomEncryptionPassphrase()
		if err != nil {
			fmt.Fprintf(os.Stderr, "rotate-encryption-key: %v\n", err)
			os.Exit(1)
		}
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	db, err := database.Open(ctx, cfg.Database.Driver, cfg.Database.Path, cfg.Database.DSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	st := store.NewWithDriver(db, cfg.Database.Driver)
	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "settings: %v\n", err)
		os.Exit(1)
	}

	result, err := mgr.RotateEncryptionKey(ctx, passphrase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rotate-encryption-key: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("re-sealed %d secret field(s)\n", result.FieldsUpdated)
	if result.KeyFileUpdated {
		fmt.Printf("updated encryption key file: %s\n", result.KeyFilePath)
	}
	if result.Note != "" {
		fmt.Printf("note: %s\n", result.Note)
	}
	// Never print the passphrase (shell history, CI logs, journald). When --generate
	// did not update the on-disk key file, write it to a 0600 temp file and print only the path.
	if *generate && !result.KeyFileUpdated {
		path, err := writeGeneratedPassphraseFile(passphrase)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rotate-encryption-key: secrets re-sealed but failed to write passphrase file: %v\nrecover the new passphrase from your process environment or re-seal from backup\n", err)
			os.Exit(1)
		}
		fmt.Printf("generated passphrase written to %s (mode 0600; copy into GITSEER_ENCRYPTION_KEY / your secret store, then delete the file)\n", path)
	}
}

// writeGeneratedPassphraseFile stores a generated encryption passphrase at 0600
// so operators can recover it without clear-text logging to stdout/stderr.
func writeGeneratedPassphraseFile(passphrase string) (string, error) {
	f, err := os.CreateTemp("", "gitseer-encryption-key-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return "", err
	}
	if _, err := f.WriteString(passphrase + "\n"); err != nil {
		_ = f.Close()
		cleanup()
		return "", err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", err
	}
	return path, nil
}

func backupCmd(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	cfgPath := fs.String("config", envOr("GITSEER_CONFIG", ""), "path to config.yaml")
	outDir := fs.String("out", "", "backup output directory")
	_ = fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	man, err := backup.Backup(context.Background(), backup.Options{
		OutDir:  *outDir,
		Version: version,
		Cfg:     cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("backup written to %s (driver=%s)\n", *outDir, man.Driver)
	for _, n := range man.Notes {
		fmt.Printf("note: %s\n", n)
	}
}

func restoreCmd(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	cfgPath := fs.String("config", envOr("GITSEER_CONFIG", ""), "path to config.yaml")
	fromDir := fs.String("from", "", "backup directory from gitseer backup")
	force := fs.Bool("force", false, "overwrite existing database / encryption key")
	_ = fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	man, err := backup.Restore(context.Background(), backup.Options{
		FromDir: *fromDir,
		Force:   *force,
		Version: version,
		Cfg:     cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "restore: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("restore complete from %s (driver=%s, backup_version=%s)\n", *fromDir, man.Driver, man.Version)
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", envOr("GITSEER_CONFIG", ""), "path to config.yaml")
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

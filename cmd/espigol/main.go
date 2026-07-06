// Command espigol launches either the admin TUI (default) or the socis HTTP
// server (--server). Configuration is resolved from $ESPIGOL_HOME.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pjover/espigol/internal/app"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/wire"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if app.ParseMode(os.Args[1:]) == app.ModeVersion {
		fmt.Println("espigol", version)
		return
	}

	home, err := config.ResolveHome()
	if err != nil {
		log.Fatalf("espigol: %v", err)
	}
	if err := config.EnsureHome(home); err != nil {
		log.Fatalf("espigol: %v", err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		log.Fatalf("espigol: %v", err)
	}
	switch app.ParseMode(os.Args[1:]) {
	case app.ModeServer:
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		srv, err := wire.Server(cfg)
		if err != nil {
			log.Fatalf("espigol server: %v", err)
		}
		if err := srv.Run(ctx); err != nil {
			log.Fatalf("espigol server: %v", err)
		}
	default:
		app, err := wire.TUI(cfg)
		if err != nil {
			log.Fatalf("espigol tui: %v", err)
		}
		if err := app.Run(); err != nil {
			log.Fatalf("espigol tui: %v", err)
		}
	}
}

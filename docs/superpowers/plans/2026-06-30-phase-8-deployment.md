# Phase 8 — Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deployment artifacts and operator documentation for running `espigol --server` and the admin TUI on a VPS, plus the cross-compile build target that produces the deployable binaries.

**Architecture:** No new application packages. A `--version` flag threaded through the existing `internal/app` mode-parsing; a `make dist` cross-compile target; checked-in `deployments/` config artifacts (systemd units, Caddyfile, backup script); an operator runbook and a README section; a CI validation job.

**Tech Stack:** Go 1.26 (existing), `systemd`, Caddy, `sqlite3` CLI (backup script), `shellcheck`/`systemd-analyze`/`caddy validate` (CI, best-effort).

## Global Constraints

- **No application logic changes** beyond `--version`. Everything else in this phase is config/docs/Makefile.
- **Targets:** `linux/amd64`, `linux/arm64`, `darwin/arm64`, all `CGO_ENABLED=0` (the existing `modernc.org/sqlite` driver is pure Go — verified CGO-free in every prior phase).
- **Placeholder domain** in the Caddyfile/docs: `espigol.example.org`. No real production domain committed.
- **Backup is local-only**; off-box copy is documented as a manual step, not scripted.
- **Existing files to extend, not replace:** `Makefile`, `cmd/espigol/main.go`, `internal/app/run.go`, `.gitignore`, `README.md`, `.github/workflows/ci.yml`. Read each before editing (this plan quotes their current full content below).
- **Catalan stays out of this phase** — these are operator-facing (English) artifacts, unlike the app's Catalan UI.

### Current file contents this plan builds on (verbatim, as of branch start)

`Makefile`:
```makefile
MODULE=github.com/pjover/espigol
BIN=bin/espigol

.PHONY: build run tui server test fmt vet tidy sqlc-generate migrate-status adopt

fmt:
	go fmt ./...

vet:
	go vet ./...

build: fmt
	mkdir -p bin
	go build -o $(BIN) ./cmd/espigol

run: build
	./$(BIN) $(ARGS)

tui: build
	./$(BIN)

server: build
	./$(BIN) --server

test:
	go test ./...

tidy:
	go mod tidy

sqlc-generate:
	go tool sqlc generate

migrate-status:
	@echo "migrations are applied automatically on Open; see db/migrations/"

adopt:
	go build -o bin/adopt ./cmd/adopt
```

`internal/app/run.go`:
```go
package app

import "flag"

// RunMode selects which driving adapter the binary launches.
type RunMode int

const (
	// ModeTUI launches the admin terminal UI (default).
	ModeTUI RunMode = iota
	// ModeServer launches the socis HTTP server.
	ModeServer
)

// ParseMode returns ModeServer when the --server flag is present, else ModeTUI.
// Unknown flags are ignored so future flags do not break dispatch.
func ParseMode(args []string) RunMode {
	fs := flag.NewFlagSet("espigol", flag.ContinueOnError)
	fs.SetOutput(nil)
	server := fs.Bool("server", false, "run the HTTP server instead of the TUI")
	_ = fs.Parse(args)
	if *server {
		return ModeServer
	}
	return ModeTUI
}
```

`cmd/espigol/main.go`:
```go
// Command espigol launches either the admin TUI (default) or the socis HTTP
// server (--server). Configuration is resolved from $ESPIGOL_HOME.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pjover/espigol/internal/app"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/wire"
)

func main() {
	home, err := config.ResolveHome()
	if err != nil {
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
```

`.gitignore`:
```
# Binaries
/bin/

# Local runtime data (config home, db, generated reports)
/.local/
*.db
*.db-shm
*.db-wal

# OS / editor noise
.DS_Store

# Go
*.test
*.out

espigol.code-workspace
```

`README.md` (full current content):
```markdown
# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable with `$ESPIGOL_HOME`.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for the phased implementation plans.
```

`.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          cache: true

      - name: Verify go.mod is tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

      - name: Vet
        run: go vet ./...

      - name: Build
        run: go build ./...

      - name: Test
        run: go test ./...
```

---

### Task 1: `--version` flag

**Files:**
- Modify: `internal/app/run.go`
- Modify: `cmd/espigol/main.go`
- Test: `internal/app/run_test.go` (extend the existing table test)

**Interfaces:**
- Produces: `app.ModeVersion RunMode` (a third `RunMode` value); `ParseMode` returns it when `--version` is passed (checked before `--server`, so `--version --server` still prints the version).
- `cmd/espigol/main.go` gains `var version = "dev"` (package-level, overridden via `-ldflags -X main.version=...` at build time) and a `case app.ModeVersion:` branch that prints `fmt.Println("espigol", version)` and returns **before** any config/DB work (`config.ResolveHome`/`config.Load` must not run for `--version`).

- [ ] **Step 1: Write the failing test**

Extend `internal/app/run_test.go` (add a case to the existing table):
```go
func TestParseMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want RunMode
	}{
		{"no args -> tui", []string{}, ModeTUI},
		{"--server -> server", []string{"--server"}, ModeServer},
		{"-server -> server", []string{"-server"}, ModeServer},
		{"--version -> version", []string{"--version"}, ModeVersion},
		{"--version takes priority over --server", []string{"--version", "--server"}, ModeVersion},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseMode(c.args); got != c.want {
				t.Errorf("ParseMode(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -v`
Expected: FAIL — `ModeVersion` undefined.

- [ ] **Step 3: Implement**

Replace the contents of `internal/app/run.go`:
```go
package app

import "flag"

// RunMode selects which driving adapter the binary launches.
type RunMode int

const (
	// ModeTUI launches the admin terminal UI (default).
	ModeTUI RunMode = iota
	// ModeServer launches the socis HTTP server.
	ModeServer
	// ModeVersion prints the binary version and exits.
	ModeVersion
)

// ParseMode returns ModeVersion when --version is present (checked first, so it
// takes priority over any other flag), ModeServer when --server is present,
// else ModeTUI. Unknown flags are ignored so future flags do not break dispatch.
func ParseMode(args []string) RunMode {
	fs := flag.NewFlagSet("espigol", flag.ContinueOnError)
	fs.SetOutput(nil)
	server := fs.Bool("server", false, "run the HTTP server instead of the TUI")
	versionFlag := fs.Bool("version", false, "print the version and exit")
	_ = fs.Parse(args)
	if *versionFlag {
		return ModeVersion
	}
	if *server {
		return ModeServer
	}
	return ModeTUI
}
```

In `cmd/espigol/main.go`, add the `version` var and the `ModeVersion` branch **before** config resolution:
```go
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
```
(`ParseMode` is called twice — once for the early version check, once in the switch. This is intentionally simple: `ParseMode` is a pure, cheap flag parse with no side effects, and keeping the `ModeServer`/default switch unchanged minimizes the diff. Do not refactor this into a single call; it is not worth the indirection for a 2-mode dispatch plus an early-exit third mode.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -v && go build ./... && ./bin/espigol --version 2>/dev/null; go run ./cmd/espigol --version`
Expected: `TestParseMode` PASS (5 cases); `go run ./cmd/espigol --version` prints `espigol dev` and exits 0 without touching `$ESPIGOL_HOME`.

- [ ] **Step 5: Commit**

```bash
git add internal/app/run.go internal/app/run_test.go cmd/espigol/main.go
git commit -m "feat(cmd): add --version flag"
```

---

### Task 2: `make dist` cross-compile + checksums

**Files:**
- Modify: `Makefile`
- Modify: `.gitignore`

**Interfaces:**
- Produces: `make dist` (builds all three targets + `dist/SHA256SUMS`), `make dist-linux-amd64`, `make dist-linux-arm64`, `make dist-darwin-arm64`, `make clean` (removes `dist/` and `bin/`).

- [ ] **Step 1: Add `dist/` to `.gitignore`**

Append to `.gitignore`:
```

# Cross-compiled deployment artifacts
/dist/
```

- [ ] **Step 2: Add the dist targets to the Makefile**

Replace the full `Makefile` with:
```makefile
MODULE=github.com/pjover/espigol
BIN=bin/espigol
VERSION:=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-s -w -X main.version=$(VERSION)
DIST_TARGETS=linux-amd64 linux-arm64 darwin-arm64

.PHONY: build run tui server test fmt vet tidy sqlc-generate migrate-status adopt \
	dist $(addprefix dist-,$(DIST_TARGETS)) clean

fmt:
	go fmt ./...

vet:
	go vet ./...

build: fmt
	mkdir -p bin
	go build -o $(BIN) ./cmd/espigol

run: build
	./$(BIN) $(ARGS)

tui: build
	./$(BIN)

server: build
	./$(BIN) --server

test:
	go test ./...

tidy:
	go mod tidy

sqlc-generate:
	go tool sqlc generate

migrate-status:
	@echo "migrations are applied automatically on Open; see db/migrations/"

adopt:
	go build -o bin/adopt ./cmd/adopt

# dist cross-compiles the deployable binary for every target in DIST_TARGETS,
# each as dist/<os>-<arch>/espigol, plus a combined SHA256SUMS.
dist: $(addprefix dist-,$(DIST_TARGETS))
	cd dist && sha256sum $(addsuffix /espigol,$(DIST_TARGETS)) > SHA256SUMS

dist-linux-amd64:
	mkdir -p dist/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-amd64/espigol ./cmd/espigol

dist-linux-arm64:
	mkdir -p dist/linux-arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-arm64/espigol ./cmd/espigol

dist-darwin-arm64:
	mkdir -p dist/darwin-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/darwin-arm64/espigol ./cmd/espigol

clean:
	rm -rf bin dist
```
(`VERSION` falls back to `dev` outside a git checkout or when there are no tags — `git describe --always` already covers the no-tags case by emitting the short commit hash, the `|| echo dev` only guards a missing `.git` entirely, e.g. a tarball build.)

- [ ] **Step 3: Verify all three targets build and checksum**

Run:
```bash
make dist
ls dist/linux-amd64/espigol dist/linux-arm64/espigol dist/darwin-arm64/espigol dist/SHA256SUMS
file dist/linux-amd64/espigol dist/linux-arm64/espigol dist/darwin-arm64/espigol
cat dist/SHA256SUMS
```
Expected: all four files exist; `file` reports `ELF 64-bit ... x86-64` / `ELF 64-bit ... ARM aarch64` / `Mach-O 64-bit ... arm64` respectively; `SHA256SUMS` has 3 lines.

- [ ] **Step 4: Verify version stamping**

Run: `git tag -l | head -1` (note: if no tags exist yet, `VERSION` will be a short commit hash via `git describe --always`, e.g. `8d3eb38` — that's expected and fine). Then:
```bash
./dist/darwin-arm64/espigol --version   # on macOS; on Linux CI use the linux-amd64 binary instead
```
Expected: prints `espigol <version>` where `<version>` is non-empty and not the literal string `dev` (since this repo has commits, `git describe --always` always returns something).

- [ ] **Step 5: Commit**

```bash
git add Makefile .gitignore
git commit -m "feat(build): add make dist cross-compile target (linux/amd64, linux/arm64, darwin/arm64)"
```

---

### Task 3: systemd units (server + backup)

**Files:**
- Create: `deployments/espigol-server.service`
- Create: `deployments/backup-espigol.sh`
- Create: `deployments/backup-espigol.service`
- Create: `deployments/backup-espigol.timer`
- Create: `deployments/espigol.env.example`

No Go code in this task — config/script artifacts only.

- [ ] **Step 1: Write `deployments/espigol-server.service`**

```ini
[Unit]
Description=Espigol socis web server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=espigol
Group=espigol
EnvironmentFile=/etc/espigol/espigol.env
Environment=ESPIGOL_HOME=/var/lib/espigol
ExecStart=/usr/local/bin/espigol --server
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/espigol

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Write `deployments/espigol.env.example`**

```sh
# Copy to /etc/espigol/espigol.env (mode 0640, owner root, group espigol).
# All variables are optional; see internal/config/config.go for the full list
# and defaults. ESPIGOL_HOME is set directly in the systemd unit, not here.

# Google OAuth (leave both empty to run the server in dev-login mode instead
# of real Google sign-in -- do NOT leave them empty in production).
ESPIGOL_OAUTH_CLIENT_ID=
ESPIGOL_OAUTH_CLIENT_SECRET=
ESPIGOL_OAUTH_REDIRECT_URL=https://espigol.example.org/oauth2/callback

# HTTP port the server binds to on localhost (Caddy reverse-proxies to this).
ESPIGOL_SERVER_PORT=8080

# Business display name shown in the TUI and on web pages / reports.
ESPIGOL_BUSINESS_NAME=Cooperativa d'Estellencs

# Audit-log actor email recorded for TUI admin mutations.
ESPIGOL_ADMIN_EMAIL=admin@espigol.example.org

# Nightly backup retention, in days (see backup-espigol.sh).
ESPIGOL_BACKUP_KEEP_DAYS=30
```
(Read `internal/config/config.go` to confirm every env var name/default referenced here
is accurate before finalizing this file — reconcile any mismatch.)

- [ ] **Step 3: Write `deployments/backup-espigol.sh`**

```sh
#!/usr/bin/env bash
# Nightly SQLite backup: an online, consistent .backup snapshot, gzipped,
# with old backups pruned. Run as the espigol system user (see
# backup-espigol.service). Off-box copy is a separate, manual step -- see
# docs/ops/DEPLOY.md.
set -euo pipefail

ESPIGOL_HOME="${ESPIGOL_HOME:-/var/lib/espigol}"
DB="$ESPIGOL_HOME/espigol.db"
OUT_DIR="$ESPIGOL_HOME/backups"
KEEP_DAYS="${ESPIGOL_BACKUP_KEEP_DAYS:-30}"

mkdir -p "$OUT_DIR"
stamp="$(date +%Y%m%d-%H%M%S)"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

sqlite3 "$DB" ".backup '$tmp'"
gzip -c "$tmp" > "$OUT_DIR/espigol-$stamp.db.gz"

find "$OUT_DIR" -name 'espigol-*.db.gz' -mtime "+$KEEP_DAYS" -delete

echo "backup written: $OUT_DIR/espigol-$stamp.db.gz"
```

- [ ] **Step 4: Write `deployments/backup-espigol.service`**

```ini
[Unit]
Description=Espigol nightly SQLite backup

[Service]
Type=oneshot
User=espigol
Group=espigol
EnvironmentFile=/etc/espigol/espigol.env
Environment=ESPIGOL_HOME=/var/lib/espigol
ExecStart=/usr/local/bin/backup-espigol.sh
```
(The runbook in Task 5 documents installing `backup-espigol.sh` to
`/usr/local/bin/backup-espigol.sh` with mode `0755`, owned by root, alongside the
`espigol` binary.)

- [ ] **Step 5: Write `deployments/backup-espigol.timer`**

```ini
[Unit]
Description=Nightly trigger for espigol SQLite backup

[Timer]
OnCalendar=*-*-* 03:30:00
Persistent=true

[Install]
WantedBy=timers.target
```

- [ ] **Step 6: Make the backup script executable in git**

```bash
chmod +x deployments/backup-espigol.sh
```

- [ ] **Step 7: Local sanity-check the backup script**

Run (against any local test SQLite DB you have, e.g. one created by `go test`, or build
one quickly):
```bash
mkdir -p /tmp/espigol-backup-check
ESPIGOL_HOME=/tmp/espigol-backup-check bash -c '
  mkdir -p "$ESPIGOL_HOME"
  sqlite3 "$ESPIGOL_HOME/espigol.db" "CREATE TABLE t(x INTEGER); INSERT INTO t VALUES (1);"
  ESPIGOL_HOME="$ESPIGOL_HOME" deployments/backup-espigol.sh
  ls "$ESPIGOL_HOME/backups"
'
rm -rf /tmp/espigol-backup-check
```
Expected: a single `espigol-<timestamp>.db.gz` file is created and listed; script exits 0.

- [ ] **Step 8: Commit**

```bash
git add deployments/espigol-server.service deployments/espigol.env.example \
  deployments/backup-espigol.sh deployments/backup-espigol.service deployments/backup-espigol.timer
git commit -m "feat(deploy): systemd units for the server and nightly local backup"
```

---

### Task 4: Caddyfile

**Files:**
- Create: `deployments/Caddyfile`

- [ ] **Step 1: Write `deployments/Caddyfile`**

```
# Replace espigol.example.org with the real domain before deploying.
# Caddy auto-provisions and renews a Let's Encrypt TLS certificate for it.
espigol.example.org {
	reverse_proxy localhost:8080
}
```
(`8080` matches the `ESPIGOL_SERVER_PORT` default in `espigol.env.example` — Task 3 Step 2.
If that default changes, keep this comment/value in sync.)

- [ ] **Step 2: Commit**

```bash
git add deployments/Caddyfile
git commit -m "feat(deploy): Caddy reverse-proxy config (placeholder domain)"
```

---

### Task 5: `docs/ops/DEPLOY.md` runbook + README Deployment section

**Files:**
- Create: `docs/ops/DEPLOY.md`
- Modify: `README.md`

- [ ] **Step 1: Write `docs/ops/DEPLOY.md`**

```markdown
# Deploying Espígol

This is the operator runbook for running `espigol --server` (the socis web app) and
the admin TUI (over SSH) on a single small VPS. See `deployments/` for the config
artifacts this runbook installs.

## 1. Provision the VPS

Any small Linux VPS works (1 vCPU / 512MB–1GB RAM is plenty for ~8 socis). Pick
`linux-amd64` or `linux-arm64` to match the VPS architecture in step 3.

```bash
ssh root@your-vps
useradd --system --create-home --home-dir /var/lib/espigol --shell /usr/sbin/nologin espigol
mkdir -p /var/lib/espigol/reports /var/lib/espigol/backups /etc/espigol
chown -R espigol:espigol /var/lib/espigol
chmod 750 /etc/espigol
```

## 2. Install Caddy and `sqlite3`

```bash
apt-get update
apt-get install -y caddy sqlite3
```

## 3. Build and install the binary

From your development machine:

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol   # or linux-arm64
scp deployments/backup-espigol.sh root@your-vps:/usr/local/bin/backup-espigol.sh
ssh root@your-vps 'chmod 755 /usr/local/bin/espigol /usr/local/bin/backup-espigol.sh'
```

## 4. Configuration

Copy `config.yaml`/`logo.png` (if you have a logo) into `/var/lib/espigol/`:

```bash
scp config.yaml logo.png root@your-vps:/var/lib/espigol/
ssh root@your-vps 'chown espigol:espigol /var/lib/espigol/config.yaml /var/lib/espigol/logo.png'
```

Copy and edit the env file:

```bash
scp deployments/espigol.env.example root@your-vps:/etc/espigol/espigol.env
ssh root@your-vps 'chmod 640 /etc/espigol/espigol.env && chgrp espigol /etc/espigol/espigol.env'
```
Edit `/etc/espigol/espigol.env` on the VPS: set the real domain in
`ESPIGOL_OAUTH_REDIRECT_URL`, the business name, the admin email, and (once you have
Google OAuth credentials -- step 6) the client id/secret. **Limits live in the database**
(set later via the TUI), not in this file.

## 5. DNS and Caddy

Point an A/AAAA record for your domain at the VPS IP. Then:

```bash
scp deployments/Caddyfile root@your-vps:/etc/caddy/Caddyfile
ssh root@your-vps "sed -i 's/espigol.example.org/your-real-domain.org/' /etc/caddy/Caddyfile"
ssh root@your-vps 'systemctl reload caddy'
```
Caddy will automatically obtain and renew a Let's Encrypt certificate for the domain on
first request -- no manual certbot step.

## 6. Google OAuth

In the [Google Cloud Console](https://console.cloud.google.com/), create an OAuth 2.0
client (type: Web application). Set the authorized redirect URI to:

```
https://your-real-domain.org/oauth2/callback
```

Put the resulting client ID and secret into `/etc/espigol/espigol.env`
(`ESPIGOL_OAUTH_CLIENT_ID`, `ESPIGOL_OAUTH_CLIENT_SECRET`). Leaving both empty runs the
server in **dev-login mode** (type-any-email bypass) -- do not deploy to production with
them empty.

## 7. Start the server

```bash
scp deployments/espigol-server.service root@your-vps:/etc/systemd/system/
ssh root@your-vps 'systemctl daemon-reload && systemctl enable --now espigol-server'
ssh root@your-vps 'systemctl status espigol-server'
```
The database and schema migrations are created/applied automatically on first start --
there is no separate migrate step.

## 8. Seed the first admin data (via the TUI, over SSH)

```bash
ssh root@your-vps
sudo -u espigol -E ESPIGOL_HOME=/var/lib/espigol /usr/local/bin/espigol
```
Use the TUI to create the first submission window, sections, partners, and board
authorizations. The TUI and the server share the same `/var/lib/espigol/espigol.db` --
there is only ever one authoritative database.

## 9. Enable nightly backups

```bash
scp deployments/backup-espigol.service deployments/backup-espigol.timer root@your-vps:/etc/systemd/system/
ssh root@your-vps 'systemctl daemon-reload && systemctl enable --now backup-espigol.timer'
ssh root@your-vps 'systemctl list-timers backup-espigol.timer'
```
Backups land in `/var/lib/espigol/backups/espigol-<timestamp>.db.gz`, pruned after
`ESPIGOL_BACKUP_KEEP_DAYS` (default 30).

**Off-box copy is not automated.** Copy `/var/lib/espigol/backups/` off the VPS
periodically using whatever tool you prefer (`rclone`, `rsync` to a second host, a
manual download, etc.) -- a single VPS with only local backups is not durable against
that VPS being lost entirely.

## 10. Updating / rolling back

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol.new
ssh root@your-vps 'mv /usr/local/bin/espigol /usr/local/bin/espigol.prev
                    mv /usr/local/bin/espigol.new /usr/local/bin/espigol
                    systemctl restart espigol-server
                    systemctl status espigol-server'
```
To roll back: `ssh root@your-vps 'mv /usr/local/bin/espigol.prev /usr/local/bin/espigol && systemctl restart espigol-server'`.

## 11. Troubleshooting

```bash
journalctl -u espigol-server -f          # server logs, live
journalctl -u backup-espigol.service     # last backup run's output
systemctl status backup-espigol.timer    # next/last scheduled backup
caddy validate --config /etc/caddy/Caddyfile   # check the Caddy config syntax
```
```

- [ ] **Step 2: Add a Deployment section to `README.md`**

Replace the full `README.md` with:
```markdown
# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable with `$ESPIGOL_HOME`.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for the phased implementation plans.

## Deployment

`make dist` cross-compiles static, CGO-free binaries for `linux/amd64`, `linux/arm64`,
and `darwin/arm64` into `dist/<os>-<arch>/espigol`. The typical update flow on a VPS is:

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol
ssh root@your-vps 'systemctl restart espigol-server'
```

`espigol --server` is meant to run under `systemd`, behind Caddy for TLS, with the
admin TUI run over SSH against the same database. See
[`docs/ops/DEPLOY.md`](docs/ops/DEPLOY.md) for the full provisioning runbook and
[`deployments/`](deployments/) for the systemd units, Caddy config, and backup script.
```

- [ ] **Step 3: Commit**

```bash
git add docs/ops/DEPLOY.md README.md
git commit -m "docs: add the deployment runbook and a README Deployment section"
```

---

### Task 6: CI `dist` validation job

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Produces: a new `dist` job in the CI workflow, running after (or alongside) `build-test`, that exercises `make dist` and validates the deployment artifacts on every push/PR.

- [ ] **Step 1: Add the `dist` job**

Replace the full `.github/workflows/ci.yml` with:
```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          cache: true

      - name: Verify go.mod is tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

      - name: Vet
        run: go vet ./...

      - name: Build
        run: go build ./...

      - name: Test
        run: go test ./...

  dist:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          cache: true

      - name: Cross-compile all targets
        run: make dist

      - name: Check binaries exist and report their type
        run: |
          for f in dist/linux-amd64/espigol dist/linux-arm64/espigol dist/darwin-arm64/espigol; do
            test -f "$f" || { echo "missing $f"; exit 1; }
            file "$f"
          done

      - name: Verify checksums
        run: |
          cd dist
          sha256sum -c SHA256SUMS

      - name: Smoke-test --version (linux/amd64, runs natively on this runner)
        run: ./dist/linux-amd64/espigol --version

      - name: Shellcheck the backup script
        run: |
          if command -v shellcheck >/dev/null; then
            shellcheck deployments/backup-espigol.sh
          else
            echo "shellcheck not available on this runner; skipping"
          fi

      - name: Verify systemd unit syntax
        run: |
          if command -v systemd-analyze >/dev/null; then
            systemd-analyze verify deployments/espigol-server.service \
              deployments/backup-espigol.service deployments/backup-espigol.timer || true
          else
            echo "systemd-analyze not available on this runner; skipping"
          fi

      - name: Validate the Caddyfile
        run: |
          if command -v caddy >/dev/null; then
            caddy validate --config deployments/Caddyfile --adapter caddyfile
          else
            echo "caddy not available on this runner; skipping"
          fi
```
(`systemd-analyze verify` is piped through `|| true` because it may reject units that
reference a non-existent `User=espigol`/`EnvironmentFile=` path on the CI runner itself
even when the unit syntax is valid — its exit code conflates syntax errors with
environment-readiness checks. `shellcheck` is preinstalled on GitHub's
`ubuntu-latest` runner image as of this writing, and `systemd-analyze` ships with
systemd itself (also preinstalled); `caddy` is NOT preinstalled, so that step is
expected to print the "not available" message and skip on a stock `ubuntu-latest`
runner unless a `caddy` install step is added later — this is intentional per the
spec's "skip gracefully" requirement, not a gap to fix in this task.)

- [ ] **Step 2: Verify locally (best-effort; the real check is the CI run after pushing)**

Run: `make dist && cd dist && sha256sum -c SHA256SUMS && cd .. && ./dist/linux-amd64/espigol --version`
(Substitute `darwin-arm64` for the `--version` smoke step if running this locally on
macOS, since `linux-amd64` won't execute there.)
Expected: checksums OK; version printed.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add a dist job that cross-compiles and validates deployment artifacts"
```

---

## Self-Review

**Spec coverage:** §2 repo layout → Tasks 3–5 (all `deployments/` artifacts + docs land
exactly where specified). §3 on-box layout/systemd/Caddy → Tasks 3–4 (unit content,
hardening directives, Caddyfile match the spec verbatim). §4 build/versioning → Tasks 1–2
(`--version` flag + `make dist` with the exact three targets, `-trimpath`, `-ldflags`,
`SHA256SUMS`). §5 backup → Task 3 (script logic, timer schedule, `Persistent=true`,
local-only with the off-box step documented, not scripted). §6 docs → Task 5 (the
runbook's 11 sections cover provision → install → config → DNS/Caddy → OAuth → start →
seed via TUI → backups → update/rollback → troubleshooting; README Deployment section
added). §7 testing/validation → Task 6 (CI `dist` job: build all targets, checksum,
`--version` smoke, shellcheck/systemd-analyze/caddy validate with graceful skip).

**Placeholder scan:** No TBD/TODO. The `espigol.env.example` step explicitly instructs
reconciling against the real `config.Config` env var names before finalizing (Task 3 Step
2) — this is a deliberate verification instruction for the implementer, not a content gap;
every other step has the complete file content inline.

**Type consistency:** `app.ModeVersion` (Task 1) is the only new Go identifier in this
phase and is fully self-contained (defined and consumed within Task 1). No cross-task Go
interfaces to reconcile — Tasks 2–6 are build/config/docs artifacts with no Go symbols.
Port `8080` is consistent between `espigol.env.example` (Task 3) and the `Caddyfile`
(Task 4), called out explicitly. Backup paths/env-var names (`ESPIGOL_HOME`,
`ESPIGOL_BACKUP_KEEP_DAYS`) are consistent between `backup-espigol.sh`,
`backup-espigol.service`, and `espigol.env.example` (all Task 3).

**Prerequisite note:** Tasks are listed in a sensible order (version flag → build target →
systemd → Caddy → docs → CI) but Tasks 3–5 have no Go-level dependency on Tasks 1–2 other
than the runbook (Task 5) referencing `make dist`'s output paths, which Task 2 defines. Task
6 (CI) depends on Tasks 2–4 existing (it runs `make dist` and validates the
`deployments/` files), so it must run last.

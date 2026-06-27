# Phase 1 — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Espígol Go repository skeleton — a single module that builds one binary which launches either a stub Bubble Tea TUI or a stub `net/http` server, with `$ESPIGOL_HOME`-based configuration, a Makefile, and CI.

**Architecture:** Hexagonal layout per golang-standards/project-layout. `cmd/espigol` parses flags and dispatches to one of two driving adapters (`internal/adapters/tui` or `internal/adapters/web`). Configuration is resolved once in `internal/config` and passed down. This phase delivers no domain logic — only the runnable shell that later phases fill in.

**Tech Stack:** Go 1.23+, `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss` (TUI), `github.com/spf13/viper` (config), Go stdlib `net/http` (server), GitHub Actions (CI).

## Global Constraints

- **Module path:** `github.com/pjover/espigol`.
- **Go version floor:** Go **1.23+** (uses `net/http` 1.22+ method-based routing). Install via `mise use go@latest` if `go` is not on PATH.
- **Single binary:** the build must stay **CGO-free** and cross-compilable. Do not add any CGO dependency in this phase.
- **Config home:** default `~/.config/espigol`, overridden by the `$ESPIGOL_HOME` environment variable. The SQLite file is always `<home>/espigol.db`.
- **All user-facing text is Catalan.** Internal logs and Go errors may be English.
- **The board is "Consell Rector"** in any Catalan string (never "junta").
- **Project layout:** `cmd/`, `internal/domain/{model,ports,services}`, `internal/adapters/{persistence,tui,web,report}`, `internal/config`, `internal/wire`, `db/` per the spec §2.2. This phase creates the directories it needs and leaves the rest for later phases.
- **TDD:** every behavioral change starts with a failing test. Commit after each green step.

---

### Task 1: Initialize module and project skeleton

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/app/doc.go`
- Create: `README.md` (overwrite the existing one-liner)

**Interfaces:**
- Consumes: nothing.
- Produces: a buildable, empty module rooted at `github.com/pjover/espigol`.

- [ ] **Step 1: Confirm Go is available**

Run: `go version`
Expected: prints `go version go1.23` or higher. If "command not found", run `mise use go@latest` then re-run.

- [ ] **Step 2: Initialize the module**

Run:
```bash
go mod init github.com/pjover/espigol
```
Expected: creates `go.mod` containing `module github.com/pjover/espigol` and a `go 1.23` (or higher) line.

- [ ] **Step 3: Write `.gitignore`**

Create `.gitignore`:
```gitignore
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
```

- [ ] **Step 4: Create a placeholder package so the module builds**

Create `internal/app/doc.go`:
```go
// Package app wires flag parsing to the espigol run modes (TUI or server).
package app
```

- [ ] **Step 5: Overwrite `README.md`**

Create `README.md`:
```markdown
# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa
d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable
with `$ESPIGOL_HOME`.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/`
for the phased implementation plans.
```

- [ ] **Step 6: Verify the module builds**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 7: Commit**

```bash
git add go.mod .gitignore internal/app/doc.go README.md
git commit -m "chore: initialize go module and project skeleton"
```

---

### Task 2: Config home resolution

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func ResolveHome() (string, error)` — returns `$ESPIGOL_HOME` if set and non-empty, otherwise `<user-home>/.config/espigol`.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"path/filepath"
	"testing"
)

func TestResolveHome_UsesEspigolHomeWhenSet(t *testing.T) {
	t.Setenv("ESPIGOL_HOME", "/custom/espigol")

	got, err := ResolveHome()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/custom/espigol" {
		t.Errorf("got %q, want %q", got, "/custom/espigol")
	}
}

func TestResolveHome_DefaultsToConfigDir(t *testing.T) {
	t.Setenv("ESPIGOL_HOME", "")
	t.Setenv("HOME", "/home/tester")

	got, err := ResolveHome()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/home/tester", ".config", "espigol")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestResolveHome -v`
Expected: FAIL — `undefined: ResolveHome`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/config.go`:
```go
// Package config resolves the espigol home directory and loads settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveHome returns the espigol home directory: $ESPIGOL_HOME when set,
// otherwise <user-home>/.config/espigol.
func ResolveHome() (string, error) {
	if h := os.Getenv("ESPIGOL_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving user home: %w", err)
	}
	return filepath.Join(home, ".config", "espigol"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestResolveHome -v`
Expected: PASS (both cases).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): resolve espigol home from ESPIGOL_HOME or ~/.config"
```

---

### Task 3: Config loading with viper

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/load_test.go`

**Interfaces:**
- Consumes: `ResolveHome()` from Task 2.
- Produces:
  - type `Config struct { Home, DBPath, BusinessName, OutputDir, BackupDir, LogoPath string; Server struct{ Port int }; OAuth struct{ ClientID, ClientSecret string } }`
  - `func Load(home string) (*Config, error)` — reads `<home>/config.yaml` if present, applies defaults, allows env overrides (prefix `ESPIGOL_`, nested keys joined with `_`). A missing config file is not an error.

- [ ] **Step 1: Add viper dependency**

Run:
```bash
go get github.com/spf13/viper@latest
```
Expected: `go.mod`/`go.sum` updated with viper.

- [ ] **Step 2: Write the failing test**

Create `internal/config/load_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	home := t.TempDir()

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Home != home {
		t.Errorf("Home = %q, want %q", cfg.Home, home)
	}
	if cfg.DBPath != filepath.Join(home, "espigol.db") {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, filepath.Join(home, "espigol.db"))
	}
	if cfg.BusinessName != "Cooperativa d'Estellencs" {
		t.Errorf("BusinessName = %q, want %q", cfg.BusinessName, "Cooperativa d'Estellencs")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.OutputDir != filepath.Join(home, "reports") {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, filepath.Join(home, "reports"))
	}
	if cfg.BackupDir != filepath.Join(home, "backups") {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, filepath.Join(home, "backups"))
	}
}

func TestLoad_ReadsYamlFile(t *testing.T) {
	home := t.TempDir()
	yaml := "" +
		"business:\n" +
		"  name: Test Coop\n" +
		"server:\n" +
		"  port: 9090\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BusinessName != "Test Coop" {
		t.Errorf("BusinessName = %q, want %q", cfg.BusinessName, "Test Coop")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ESPIGOL_SERVER_PORT", "7000")

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 7000 {
		t.Errorf("Server.Port = %d, want 7000", cfg.Server.Port)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — `undefined: Load` and `undefined: Config`.

- [ ] **Step 4: Write minimal implementation**

Append to `internal/config/config.go` (and add the new imports `strings` and `github.com/spf13/viper` to the existing import block):
```go
// Config holds the resolved espigol settings.
type Config struct {
	Home         string
	DBPath       string
	BusinessName string
	OutputDir    string
	BackupDir    string
	LogoPath     string
	Server       struct {
		Port int
	}
	OAuth struct {
		ClientID     string
		ClientSecret string
	}
}

// Load reads <home>/config.yaml if present, applies defaults, and allows
// environment overrides (prefix ESPIGOL_, nested keys joined with "_").
// A missing config file is not an error.
func Load(home string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(home)

	v.SetEnvPrefix("ESPIGOL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("business.name", "Cooperativa d'Estellencs")
	v.SetDefault("server.port", 8080)
	v.SetDefault("output.dir", filepath.Join(home, "reports"))
	v.SetDefault("backup.dir", filepath.Join(home, "backups"))
	v.SetDefault("logo.path", filepath.Join(home, "logo.png"))
	v.SetDefault("oauth.client_id", "")
	v.SetDefault("oauth.client_secret", "")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errorsAs(err, &notFound) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	cfg := &Config{
		Home:         home,
		DBPath:       filepath.Join(home, "espigol.db"),
		BusinessName: v.GetString("business.name"),
		OutputDir:    v.GetString("output.dir"),
		BackupDir:    v.GetString("backup.dir"),
		LogoPath:     v.GetString("logo.path"),
	}
	cfg.Server.Port = v.GetInt("server.port")
	cfg.OAuth.ClientID = v.GetString("oauth.client_id")
	cfg.OAuth.ClientSecret = v.GetString("oauth.client_secret")
	return cfg, nil
}
```

Add this small helper at the bottom of the file (keeps the `errors` import local and explicit):
```go
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
```

Update the import block at the top of `internal/config/config.go` to:
```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all `ResolveHome` and `Load` tests).

- [ ] **Step 6: Tidy and verify build**

Run: `go mod tidy && go build ./...`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/load_test.go go.mod go.sum
git commit -m "feat(config): load config.yaml with defaults and env overrides"
```

---

### Task 4: Run-mode flag parsing

**Files:**
- Create: `internal/app/run.go`
- Test: `internal/app/run_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - type `RunMode int` with constants `ModeTUI RunMode = iota` and `ModeServer`.
  - `func ParseMode(args []string) RunMode` — returns `ModeServer` when `--server` (or `-server`) is present, otherwise `ModeTUI`.

- [ ] **Step 1: Write the failing test**

Create `internal/app/run_test.go`:
```go
package app

import "testing"

func TestParseMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want RunMode
	}{
		{"no args -> tui", []string{}, ModeTUI},
		{"--server -> server", []string{"--server"}, ModeServer},
		{"-server -> server", []string{"-server"}, ModeServer},
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

Run: `go test ./internal/app/ -run TestParseMode -v`
Expected: FAIL — `undefined: RunMode` / `undefined: ParseMode`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/app/run.go`:
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app/ -run TestParseMode -v`
Expected: PASS (all three cases).

- [ ] **Step 5: Commit**

```bash
git add internal/app/run.go internal/app/run_test.go
git commit -m "feat(app): parse --server flag into a run mode"
```

---

### Task 5: Stub HTTP server with health endpoint and graceful shutdown

**Files:**
- Create: `internal/adapters/web/server.go`
- Test: `internal/adapters/web/server_test.go`

**Interfaces:**
- Consumes: `*config.Config` from Task 3.
- Produces:
  - `func NewHandler(cfg *config.Config) http.Handler` — returns the mux with `GET /health`.
  - `func Run(ctx context.Context, cfg *config.Config) error` — starts the server and shuts it down gracefully when `ctx` is cancelled.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/web/server_test.go`:
```go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pjover/espigol/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	h := NewHandler(&config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "OK\n" {
		t.Errorf("body = %q, want %q", got, "OK\n")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/web/ -run TestHealthEndpoint -v`
Expected: FAIL — `undefined: NewHandler`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/adapters/web/server.go`:
```go
// Package web is the socis-facing HTTP driving adapter. In phase 1 it only
// exposes a health endpoint; routes and auth arrive in a later phase.
package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pjover/espigol/internal/config"
)

// NewHandler builds the HTTP handler tree.
func NewHandler(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	return mux
}

// Run starts the HTTP server and shuts it down gracefully when ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: NewHandler(cfg),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("espigol server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/web/ -run TestHealthEndpoint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/web/server.go internal/adapters/web/server_test.go
git commit -m "feat(web): stub server with /health and graceful shutdown"
```

---

### Task 6: Stub Bubble Tea TUI

**Files:**
- Create: `internal/adapters/tui/tui.go`
- Test: `internal/adapters/tui/tui_test.go`

**Interfaces:**
- Consumes: `*config.Config` from Task 3.
- Produces:
  - `func NewModel(cfg *config.Config) Model` — the initial Bubble Tea model.
  - `Model` implements `tea.Model` (`Init`, `Update`, `View`); pressing `q` or `ctrl+c` returns `tea.Quit`; `View()` contains the Catalan quit hint.
  - `func Run(cfg *config.Config) error` — runs the Bubble Tea program.

- [ ] **Step 1: Add Bubble Tea + Lip Gloss dependencies**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
```
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/tui/tui_test.go`:
```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pjover/espigol/internal/config"
)

func TestView_ShowsCatalanQuitHint(t *testing.T) {
	m := NewModel(&config.Config{})
	if !strings.Contains(m.View(), "prem q per sortir") {
		t.Errorf("View() = %q, want it to contain the Catalan quit hint", m.View())
	}
}

func TestUpdate_QuitsOnQ(t *testing.T) {
	m := NewModel(&config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a command on 'q', got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestUpdate_QuitsOnCtrlC(t *testing.T) {
	m := NewModel(&config.Config{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command on ctrl+c, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/tui/ -v`
Expected: FAIL — `undefined: NewModel`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/adapters/tui/tui.go`:
```go
// Package tui is the admin-facing terminal UI driving adapter. In phase 1 it
// is a minimal Bubble Tea program; panels and keymaps arrive in a later phase.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/config"
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg *config.Config
}

// NewModel builds the initial model.
func NewModel(cfg *config.Config) Model {
	return Model{cfg: cfg}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

var titleStyle = lipgloss.NewStyle().Bold(true)

// View implements tea.Model.
func (m Model) View() string {
	return titleStyle.Render("Espígol") + "\n\nprem q per sortir\n"
}

// Run starts the Bubble Tea program.
func Run(cfg *config.Config) error {
	_, err := tea.NewProgram(NewModel(cfg)).Run()
	return err
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/adapters/tui/ -v`
Expected: PASS (all three cases).

- [ ] **Step 6: Tidy and verify build**

Run: `go mod tidy && go build ./...`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/tui/tui.go internal/adapters/tui/tui_test.go go.mod go.sum
git commit -m "feat(tui): stub bubble tea model that quits on q/ctrl+c"
```

---

### Task 7: Wire the entrypoint

**Files:**
- Create: `cmd/espigol/main.go`

**Interfaces:**
- Consumes: `config.ResolveHome`, `config.Load` (Tasks 2–3); `app.ParseMode`, `app.ModeServer` (Task 4); `web.Run` (Task 5); `tui.Run` (Task 6).
- Produces: the `main` package — the runnable binary.

- [ ] **Step 1: Write the entrypoint**

Create `cmd/espigol/main.go`:
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

	"github.com/pjover/espigol/internal/adapters/tui"
	"github.com/pjover/espigol/internal/adapters/web"
	"github.com/pjover/espigol/internal/app"
	"github.com/pjover/espigol/internal/config"
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
		if err := web.Run(ctx, cfg); err != nil {
			log.Fatalf("espigol server: %v", err)
		}
	default:
		if err := tui.Run(cfg); err != nil {
			log.Fatalf("espigol tui: %v", err)
		}
	}
}
```

- [ ] **Step 2: Verify the whole module builds**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Build the binary and smoke-test the server**

Run:
```bash
go build -o bin/espigol ./cmd/espigol
ESPIGOL_HOME=$(mktemp -d) ESPIGOL_SERVER_PORT=8099 ./bin/espigol --server &
sleep 1
curl -s localhost:8099/health
kill %1
```
Expected: prints `OK`, then the server is killed. (The `&` backgrounds the server; `kill %1` stops it.)

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS (config, app, web, tui).

- [ ] **Step 5: Commit**

```bash
git add cmd/espigol/main.go
git commit -m "feat: wire entrypoint to dispatch TUI or server"
```

---

### Task 8: Makefile

**Files:**
- Create: `Makefile`

**Interfaces:**
- Consumes: the package layout from earlier tasks.
- Produces: `make` targets `build`, `run`, `tui`, `server`, `test`, `fmt`, `vet`, `tidy`.

- [ ] **Step 1: Write the Makefile**

Create `Makefile`:
```makefile
MODULE=github.com/pjover/espigol
BIN=bin/espigol

.PHONY: build run tui server test fmt vet tidy

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
```

- [ ] **Step 2: Verify the targets work**

Run: `make build && make vet && make test`
Expected: builds `bin/espigol`, `go vet` is clean, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add Makefile with build/run/test/vet targets"
```

---

### Task 9: Continuous integration

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the Makefile targets (Task 8).
- Produces: a GitHub Actions workflow running build, vet, and tests on push and PR.

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/ci.yml`:
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
          go-version: '1.23'
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

- [ ] **Step 2: Verify the workflow file is valid YAML**

Run: `cat .github/workflows/ci.yml`
Expected: the file prints with the structure above (no tabs — YAML uses spaces).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: build, vet, and test on push and PR"
```

---

## Self-Review

**Spec coverage (Phase 1 scope from §10 + §2.2, §9.1, §9.2):**
- Go module + golang-standards/project-layout → Task 1 (+ later tasks create `internal/config`, `internal/app`, `internal/adapters/{web,tui}`; remaining layout dirs are created as later phases need them, per Global Constraints).
- `cmd/espigol` entrypoint with `--server` dispatch → Tasks 4 (parse) + 7 (wire).
- Config: `$ESPIGOL_HOME` resolution + viper + `config.yaml` (incl. backup dir from the spec edit) → Tasks 2–3.
- Stub TUI (Bubble Tea) starting/exiting cleanly → Task 6.
- Stub server (`net/http`) starting/exiting cleanly → Tasks 5 + 7 (graceful shutdown on SIGINT/SIGTERM).
- Makefile → Task 8. CI → Task 9.

**Placeholder scan:** No "TBD"/"add error handling here"/"similar to Task N" placeholders; every code step contains complete, compilable Go.

**Type consistency:** `config.Config` (with `Server.Port`, `OAuth.ClientID/ClientSecret`, `Home`, `DBPath`, `OutputDir`, `BackupDir`, `LogoPath`) is defined in Task 3 and consumed verbatim in Tasks 5–7. `app.ParseMode`/`app.ModeServer` (Task 4) are consumed in Task 7. `web.Run(ctx, cfg)` (Task 5) and `tui.Run(cfg)` (Task 6) match their call sites in Task 7. `NewHandler`/`NewModel` signatures match their tests.

**Notes for later phases (out of Phase 1 scope, recorded so they aren't lost):**
- The data-driven sections model, `Money`, and the SQLite schema land in Phase 2.
- The `ExpenseForecast.id` `CPYYnnn` generation rule and the adopt-the-Java-DB transform land in Phase 2.

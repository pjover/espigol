# Phase 6 — Server (socis web) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A socis-only web server — SQLite-backed sessions, Google OAuth with an auto dev-login bypass, a new `ForecastService` for soci/board forecast CRUD, a read-only HTML report view reusing the shared `buildLayout`, and the first real production wiring replacing the Phase-1 stub.

**Architecture:** `application.ForecastService` (over `TxManager`) owns soci/board CRUD + authorization. `internal/adapters/auth` holds the SQLite `SessionStore`, the authenticator (OAuth/dev), and `RequireAuth` middleware. `internal/adapters/web` holds `net/http` handlers + `html/template` pages. `internal/adapters/report` gains an `HTMLRenderer`. `internal/wire` assembles it all for `cmd/espigol --server`.

**Tech Stack:** Go 1.26, `net/http` (1.22 routing), `html/template`, `golang.org/x/oauth2` (+ google), `modernc.org/sqlite`, sqlc, stdlib.

## Global Constraints

- **Module:** `github.com/pjover/espigol`. Go 1.26. CGO-free.
- **Socis-only server:** never import `application.WindowService` or any admin op. The server only *reads* the published `Report` (`ReportRepository.FindLatestByYear`).
- **Layering:** `web` (driving adapter) may depend on `application` + `auth` + `report` + `ports`. `application` depends only on `ports`/`model` (+ stdlib). `auth` and `report` are adapters: domain + stdlib (+ sqlc for the session store; + x/oauth2 for the authenticator). No `float64`.
- **All user-facing text Catalan** (templates, error messages, access-denied). Internal logs/errors English.
- **Auth mode auto-detect:** dev when `cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == ""` (register `/login` form + `POST /dev-login`); else prod Google OAuth (no `/dev-login`). Cookie `Secure` = !dev.
- **Session cookie:** name `espigol_session`, opaque 32-byte base64url token, `HttpOnly`, `SameSite=Lax`, `Path=/`, TTL 30 days.
- **CSRF:** POST-only mutations; a per-session CSRF token in a hidden form field, verified on POST.
- **Existing signatures (verbatim):**
  - `ports.TxManager.WithinTx(ctx, func(ports.RepoSet) error) error`; `RepoSet{Partners, Forecasts, Windows, Taxonomy, Sections, Reports, Audit}`.
  - `ports.ForecastRepository`: `Create(ctx, f) (f, error)` (allocates CPYYnnn), `Save(ctx, f) error`, `FindByID(ctx, id string) (f, bool, error)`, `ListByYear(ctx, year) ([]f, error)`.
  - `ports.PartnerRepository`: `FindByID(ctx,int)(Partner,bool,error)`, `FindByEmail(ctx,string)(Partner,bool,error)`, `List`.
  - `ports.WindowRepository.List`, `.FindByYear`. `ports.BoardAuthorizationRepository.ListByPartner(ctx,int)`. `ports.ReportRepository.FindLatestByYear(ctx,int)(Report,bool,error)`. `ports.TaxonomyRepository.ListSubtypes(ctx,year)`/`ListTypes`. `ports.SectionRepository.List`/`ListMemberships`.
  - `model.Partner`: `ID() int`, `Email() string`, `Name()`, `BoardMember() bool`.
  - `model.NewExpenseForecast(id string, partnerID int, concept, description string, gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int, subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error)`; `NewUnsavedExpenseForecast(...same minus id...)`.
  - `model.NewCommonScope()`, `NewPartnerScope()`, `NewSectionScope(code) (ExpenseScope,error)`; `model.ScopeCommon/ScopeSection/ScopePartner`; `ExpenseScope.Kind()`, `.SectionCode()`.
  - `model.BoardAuthorization`: `ScopeKind() ScopeKind`, `SectionCode() string`.
  - `model.SubmissionWindow`: `Year()`, `State()`, `Deadline()`, `CurrentExpenseLimit()`, `InvestmentExpenseLimit()`; `model.WindowOpen`, `WindowClosed`.
  - `model.NewAuditEvent(id int, actorID *int, actorEmail string, kind AuditKind, entityType, entityID string, ts time.Time, payload *string)`; `model.AuditForecastCreated/Edited/Deleted`.
  - `model.MoneyFromString`, `Money.String()`.
  - `application` package-private helpers reusable from new files in the same package: `appendAudit(ctx, r ports.RepoSet, kind, entityType, entityID string, at time.Time, payload string) error` (system actor — NOT for forecast events), `mostRecentPrior`. `application.SnapshotFromJSON(string)(report.ReportData,error)`.
  - `report.HTMLRenderer` (Task 4); `report` `buildLayout` is package-private (HTMLRenderer is in the same `report` package).
  - `config.Config` fields `Server.Port`, `OAuth.ClientID/ClientSecret`, `BusinessName`, `DBPath`; `db.Open(path)(*sql.DB,error)`; `sqlc.New(db)`.
- **TDD:** failing test first; commit after each green step.

---

### Task 1: Config — OAuth.RedirectURL

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/oauth_test.go`

**Interfaces:**
- Produces: `Config.OAuth.RedirectURL string` (default `""`; env `ESPIGOL_OAUTH_REDIRECT_URL`).

- [ ] **Step 1: Write the failing test**

Create `internal/config/oauth_test.go`:
```go
package config

import "testing"

func TestLoad_OAuthRedirectURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ESPIGOL_OAUTH_REDIRECT_URL", "https://espigol.example/oauth2/callback")
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuth.RedirectURL != "https://espigol.example/oauth2/callback" {
		t.Errorf("RedirectURL = %q", cfg.OAuth.RedirectURL)
	}
}

func TestLoad_OAuthRedirectURL_DefaultsEmpty(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuth.RedirectURL != "" {
		t.Errorf("RedirectURL default = %q, want empty", cfg.OAuth.RedirectURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_OAuthRedirectURL -v`
Expected: FAIL — `RedirectURL` field undefined.

- [ ] **Step 3: Add the field + default + binding**

In `internal/config/config.go`: add `RedirectURL string` to the `OAuth` anonymous struct; add `v.SetDefault("oauth.redirect_url", "")` beside the other oauth defaults; add `cfg.OAuth.RedirectURL = v.GetString("oauth.redirect_url")` beside the other oauth assignments.

- [ ] **Step 4: Run test + build**

Run: `go test ./internal/config/ -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/oauth_test.go
git commit -m "feat(config): add OAuth.RedirectURL"
```

---

### Task 2: Session store (migration + auth.SessionStore)

**Files:**
- Create: `db/migrations/00002_session.sql`
- Create: `db/queries/session.sql`
- Create: `internal/adapters/auth/session.go`
- Test: `internal/adapters/auth/session_test.go`

**Interfaces:**
- Produces:
  - `type Session struct { Token string; PartnerID int; Email string; ExpiresAt time.Time }`
  - `type SessionStore struct { ... }`; `NewSessionStore(q *sqlc.Queries, clock ports.Clock) *SessionStore`
  - `Create(ctx, partnerID int, email string) (string, error)` (random token, 30-day TTL)
  - `Get(ctx, token string) (Session, bool, error)` (only non-expired)
  - `Delete(ctx, token string) error`

- [ ] **Step 1: Write the migration**

Create `db/migrations/00002_session.sql`:
```sql
-- +goose Up
CREATE TABLE session (
    token      TEXT PRIMARY KEY,
    partner_id INTEGER NOT NULL,
    email      TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    FOREIGN KEY (partner_id) REFERENCES partner(id)
);

CREATE INDEX idx_session_expires ON session(expires_at);

-- +goose Down
DROP TABLE session;
```

- [ ] **Step 2: Write the queries and generate**

Create `db/queries/session.sql`:
```sql
-- name: InsertSession :exec
INSERT INTO session (token, partner_id, email, created_at, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT token, partner_id, email, created_at, expires_at
FROM session WHERE token = ?;

-- name: DeleteSession :exec
DELETE FROM session WHERE token = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM session WHERE expires_at < ?;
```

Run: `make sqlc-generate`
Expected: generates `Session` row + query methods. Read the generated types (`expires_at`/`created_at` are TEXT → `string`; `partner_id` → `int64`).

- [ ] **Step 3: Write the failing test**

Create `internal/adapters/auth/session_test.go`:
```go
package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/adapters/persistence"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newStore(t *testing.T, now time.Time) (*auth.SessionStore, *sqlc.Queries) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	// a partner for the FK
	p, _ := model.NewPartner(1, "Soci", "", "", "s1@e.test", "", model.Productor, 0,
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err := persistence.NewPartnerRepository(q).Save(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return auth.NewSessionStore(q, fixedClock{t: now}), q
}

func TestSessionStore_CreateGetDelete(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, _ := newStore(t, now)
	ctx := context.Background()

	token, err := store.Create(ctx, 1, "s1@e.test")
	if err != nil || token == "" {
		t.Fatalf("create: token=%q err=%v", token, err)
	}
	s, ok, err := store.Get(ctx, token)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if s.PartnerID != 1 || s.Email != "s1@e.test" {
		t.Errorf("session mismatch: %+v", s)
	}
	if err := store.Delete(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(ctx, token); ok {
		t.Error("session should be gone after delete")
	}
}

func TestSessionStore_ExpiredIsAbsent(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, _ := newStore(t, now)
	ctx := context.Background()
	token, _ := store.Create(ctx, 1, "s1@e.test")

	// advance the store's clock past the TTL by making a new store at now+31d sharing the DB
	// simplest: read with a future-clock store
	future := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	store.SetNow(future) // test helper on the store; see implementation note
	if _, ok, _ := store.Get(ctx, token); ok {
		t.Error("expired session should be treated as absent")
	}
}
```
Implementation note: rather than a `SetNow` setter, the store reads `clock.Now()` each call — so for the expiry test, construct the store with a mutable clock. Replace `store.SetNow(future)` by using a pointer clock: define `type mutClock struct{ t time.Time }; func (c *mutClock) Now() time.Time { return c.t }`, build the store with `&mutClock{now}`, and in the test set `mc.t = future` before the second `Get`. Adjust `newStore` to return the `*mutClock` too. (Do not add a `SetNow` method to production code.)

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/adapters/auth/ -v`
Expected: FAIL — undefined `auth.SessionStore`.

- [ ] **Step 5: Write the store**

Create `internal/adapters/auth/session.go`:
```go
// Package auth provides the socis server's session store, authenticator
// (Google OAuth or dev-login), and the RequireAuth middleware.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/ports"
)

const sessionTTL = 30 * 24 * time.Hour

// Session is an authenticated session.
type Session struct {
	Token     string
	PartnerID int
	Email     string
	ExpiresAt time.Time
}

// SessionStore persists sessions in SQLite.
type SessionStore struct {
	q     *sqlc.Queries
	clock ports.Clock
}

func NewSessionStore(q *sqlc.Queries, clock ports.Clock) *SessionStore {
	return &SessionStore{q: q, clock: clock}
}

// Create issues a new session token for the partner with a 30-day TTL.
func (s *SessionStore) Create(ctx context.Context, partnerID int, email string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	now := s.clock.Now().UTC()
	err := s.q.InsertSession(ctx, sqlc.InsertSessionParams{
		Token:     token,
		PartnerID: int64(partnerID),
		Email:     email,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(sessionTTL).Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Get returns the session for a token if it exists and has not expired.
func (s *SessionStore) Get(ctx context.Context, token string) (Session, bool, error) {
	row, err := s.q.GetSession(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	exp, err := time.Parse(time.RFC3339, row.ExpiresAt)
	if err != nil {
		return Session{}, false, err
	}
	if !exp.After(s.clock.Now().UTC()) {
		return Session{}, false, nil // expired
	}
	return Session{Token: row.Token, PartnerID: int(row.PartnerID), Email: row.Email, ExpiresAt: exp}, true, nil
}

// Delete removes a session (logout).
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	return s.q.DeleteSession(ctx, token)
}
```
(If the generated param/row field names differ — e.g. `CreatedAt`/`ExpiresAt`/`PartnerID` — reconcile to the actual sqlc identifiers.)

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/auth/... -v && go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/00002_session.sql db/queries/session.sql internal/adapters/persistence/sqlc/ internal/adapters/auth/session.go internal/adapters/auth/session_test.go
git commit -m "feat(auth): SQLite session store + session migration"
```

---

### Task 3: application.ForecastService

**Files:**
- Modify: `internal/application/errors.go`
- Create: `internal/application/forecast_service.go`
- Test: `internal/application/forecast_service_test.go`

**Interfaces:**
- Produces:
  - errors: `ErrNoOpenWindow`, `ErrWindowNotOpen`, `ErrForbidden`, `ErrForecastNotFound`.
  - `type ForecastInput struct { Concept, Description string; GrossAmount model.Money; PlannedDate time.Time; SubtypeCode string; ScopeKind model.ScopeKind; SectionCode string }`
  - `type DashboardView struct { Year int; Deadline time.Time; Mine []model.ExpenseForecast; BoardScoped []model.ExpenseForecast; ClosedYears []int }`
  - `type ForecastService struct { ... }`; `NewForecastService(tx ports.TxManager, clock ports.Clock) *ForecastService`
  - `Dashboard(ctx, actor model.Partner) (DashboardView, error)`
  - `Create(ctx, actor model.Partner, in ForecastInput) (model.ExpenseForecast, error)`
  - `Update(ctx, actor model.Partner, id string, in ForecastInput) error`
  - `Delete(ctx, actor model.Partner, id string) error`
  - `Get(ctx, actor model.Partner, id string) (model.ExpenseForecast, error)`

- [ ] **Step 1: Extend errors.go**

Add to `internal/application/errors.go` `var (...)`:
```go
	ErrNoOpenWindow     = errors.New("no submission window is currently open")
	ErrWindowNotOpen    = errors.New("el termini ja ha finalitzat, contacta amb el Consell Rector")
	ErrForbidden        = errors.New("not authorized to act on this forecast scope")
	ErrForecastNotFound = errors.New("forecast not found")
```

- [ ] **Step 2: Write the failing test**

Create `internal/application/forecast_service_test.go`:
```go
package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
	"database/sql"
	"path/filepath"
)

func fcNow() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

func newFcSvc(t *testing.T) (*application.ForecastService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "fc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return application.NewForecastService(persistence.NewTxManager(conn), fixedClock{t: fcNow()}), conn
}

// seedOpen2026 builds an OPEN 2026 window with taxonomy a1(CURRENT)/b1(INVESTMENT),
// sections oliva/ramaderia, and partners 1 (soci) and 7 (board, authorized COMMON + SECTION oliva).
func seedOpen2026(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(fcNow()), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = persistence.NewWindowRepository(q).Save(ctx, w)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	sr := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)
	pr := persistence.NewPartnerRepository(q)
	soci, _ := model.NewPartner(1, "Soci U", "", "", "u1@e.test", "", model.Productor, 0, fcNow(), false)
	board, _ := model.NewPartner(7, "Board", "", "", "b7@e.test", "", model.Productor, 0, fcNow(), true)
	_ = pr.Save(ctx, soci)
	_ = pr.Save(ctx, board)
	bar := persistence.NewBoardAuthorizationRepository(q)
	ac, _ := model.NewBoardAuthorization(7, model.ScopeCommon, "")
	as, _ := model.NewBoardAuthorization(7, model.ScopeSection, "oliva")
	_ = bar.Save(ctx, ac)
	_ = bar.Save(ctx, as)
}

func partner(t *testing.T, conn *sql.DB, id int) model.Partner {
	t.Helper()
	p, ok, err := persistence.NewPartnerRepository(sqlc.New(conn)).FindByID(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("partner %d: %v", id, err)
	}
	return p
}

func partnerInput(amount string) application.ForecastInput {
	m, _ := model.MoneyFromString(amount)
	return application.ForecastInput{
		Concept: "Eines", Description: "", GrossAmount: m,
		PlannedDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		SubtypeCode: "b1", ScopeKind: model.ScopePartner,
	}
}

func TestForecastService_SociCreateAndOwnEdit(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	soci := partner(t, conn, 1)

	f, err := svc.Create(ctx, soci, partnerInput("500.00"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if f.PartnerID() != 1 || f.Scope().Kind() != model.ScopePartner || f.ID() == "" {
		t.Errorf("created wrong: %+v", f)
	}
	in := partnerInput("600.00")
	if err := svc.Update(ctx, soci, f.ID(), in); err != nil {
		t.Fatalf("update own: %v", err)
	}
	got, _ := svc.Get(ctx, soci, f.ID())
	if got.GrossAmount().String() != "600.00" {
		t.Errorf("update not applied: %s", got.GrossAmount())
	}
	if err := svc.Delete(ctx, soci, f.ID()); err != nil {
		t.Fatalf("delete own: %v", err)
	}
}

func TestForecastService_SociCannotTouchOthers(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	board := partner(t, conn, 7)
	soci := partner(t, conn, 1)

	// board creates a COMMON forecast it is authorized for
	common := application.ForecastInput{Concept: "Comú", GrossAmount: mustMoney(t, "100.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1", ScopeKind: model.ScopeCommon}
	cf, err := svc.Create(ctx, board, common)
	if err != nil {
		t.Fatalf("board create common: %v", err)
	}
	// a regular soci must not edit a COMMON forecast
	if err := svc.Update(ctx, soci, cf.ID(), common); !errors.Is(err, application.ErrForbidden) {
		t.Errorf("soci editing COMMON: want ErrForbidden, got %v", err)
	}
}

func TestForecastService_BoardScopeAuthorization(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	board := partner(t, conn, 7)

	// authorized: SECTION oliva
	okIn := application.ForecastInput{Concept: "Oliva", GrossAmount: mustMoney(t, "200.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1",
		ScopeKind: model.ScopeSection, SectionCode: "oliva"}
	if _, err := svc.Create(ctx, board, okIn); err != nil {
		t.Fatalf("board create authorized section: %v", err)
	}
	// unauthorized: SECTION ramaderia (board 7 only authorized for oliva + common)
	badIn := okIn
	badIn.SectionCode = "ramaderia"
	if _, err := svc.Create(ctx, board, badIn); !errors.Is(err, application.ErrForbidden) {
		t.Errorf("board create unauthorized section: want ErrForbidden, got %v", err)
	}
}

func TestForecastService_RejectsWhenNoOpenWindow(t *testing.T) {
	svc, conn := newFcSvc(t)
	// no window seeded
	_ = conn
	soci, _ := model.NewPartner(1, "X", "", "", "x@e.test", "", model.Productor, 0, fcNow(), false)
	if _, err := svc.Create(context.Background(), soci, partnerInput("100.00")); !errors.Is(err, application.ErrNoOpenWindow) {
		t.Errorf("want ErrNoOpenWindow, got %v", err)
	}
}
```
(`ptrTime`, `mustMoney`, `fixedClock` already exist in the application test package from Phase 4 tests.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/application/ -run TestForecastService -v`
Expected: FAIL — undefined `NewForecastService`.

- [ ] **Step 4: Write the service**

Create `internal/application/forecast_service.go`:
```go
package application

import (
	"context"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// ForecastInput is the form data for creating/updating a forecast.
type ForecastInput struct {
	Concept     string
	Description string
	GrossAmount model.Money
	PlannedDate time.Time
	SubtypeCode string
	ScopeKind   model.ScopeKind
	SectionCode string
}

// DashboardView is the soci dashboard data.
type DashboardView struct {
	Year        int
	Deadline    time.Time
	Mine        []model.ExpenseForecast
	BoardScoped []model.ExpenseForecast
	ClosedYears []int
}

// ForecastService is the socis-facing forecast CRUD service.
type ForecastService struct {
	tx    ports.TxManager
	clock ports.Clock
}

func NewForecastService(tx ports.TxManager, clock ports.Clock) *ForecastService {
	return &ForecastService{tx: tx, clock: clock}
}

func openWindow(ctx context.Context, r ports.RepoSet) (model.SubmissionWindow, error) {
	all, err := r.Windows.List(ctx)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	for _, w := range all {
		if w.State() == model.WindowOpen {
			return w, nil
		}
	}
	return model.SubmissionWindow{}, ErrNoOpenWindow
}

// authorizeScope checks that the actor may act on a forecast of (scope, partnerID).
func authorizeScope(ctx context.Context, r ports.RepoSet, actor model.Partner, scope model.ExpenseScope, ownerID int) error {
	switch scope.Kind() {
	case model.ScopePartner:
		if ownerID != actor.ID() {
			return ErrForbidden
		}
		return nil
	case model.ScopeCommon, model.ScopeSection:
		if !actor.BoardMember() {
			return ErrForbidden
		}
		auths, err := r.BoardAuth.ListByPartner(ctx, actor.ID())
		if err != nil {
			return err
		}
		for _, a := range auths {
			if a.ScopeKind() == scope.Kind() && a.SectionCode() == scope.SectionCode() {
				return nil
			}
		}
		return ErrForbidden
	default:
		return ErrForbidden
	}
}

func buildScope(in ForecastInput) (model.ExpenseScope, error) {
	switch in.ScopeKind {
	case model.ScopeCommon:
		return model.NewCommonScope(), nil
	case model.ScopePartner:
		return model.NewPartnerScope(), nil
	case model.ScopeSection:
		return model.NewSectionScope(in.SectionCode)
	default:
		return model.ExpenseScope{}, ErrForbidden
	}
}

func (s *ForecastService) Create(ctx context.Context, actor model.Partner, in ForecastInput) (model.ExpenseForecast, error) {
	now := s.clock.Now()
	var created model.ExpenseForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		scope, err := buildScope(in)
		if err != nil {
			return err
		}
		// the owner for PARTNER scope is the actor; for COMMON/SECTION it is also the actor (the entering board member)
		if err := authorizeScope(ctx, r, actor, scope, actor.ID()); err != nil {
			return err
		}
		f, err := model.NewUnsavedExpenseForecast(actor.ID(), in.Concept, in.Description,
			in.GrossAmount, model.ZeroMoney(), nil, in.PlannedDate, w.Year(), in.SubtypeCode, scope, now, true)
		if err != nil {
			return err
		}
		saved, err := r.Forecasts.Create(ctx, f)
		if err != nil {
			return err
		}
		created = saved
		return forecastAudit(ctx, r, actor, model.AuditForecastCreated, saved.ID(), now)
	})
	return created, err
}

func (s *ForecastService) Update(ctx context.Context, actor model.Partner, id string, in ForecastInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		existing, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok || existing.Year() != w.Year() {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, existing.Scope(), existing.PartnerID()); err != nil {
			return err
		}
		scope, err := buildScope(in)
		if err != nil {
			return err
		}
		// the new scope must also be one the actor may use
		if err := authorizeScope(ctx, r, actor, scope, existing.PartnerID()); err != nil {
			return err
		}
		updated, err := model.NewExpenseForecast(id, existing.PartnerID(), in.Concept, in.Description,
			in.GrossAmount, existing.ApprovedAmount(), existing.ApprovedOn(), in.PlannedDate, w.Year(),
			in.SubtypeCode, scope, existing.AddedOn(), existing.Enabled())
		if err != nil {
			return err
		}
		if err := r.Forecasts.Save(ctx, updated); err != nil {
			return err
		}
		return forecastAudit(ctx, r, actor, model.AuditForecastEdited, id, now)
	})
}

func (s *ForecastService) Delete(ctx context.Context, actor model.Partner, id string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		existing, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok || existing.Year() != w.Year() {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, existing.Scope(), existing.PartnerID()); err != nil {
			return err
		}
		if err := r.Forecasts.Delete(ctx, id); err != nil {
			return err
		}
		return forecastAudit(ctx, r, actor, model.AuditForecastDeleted, id, now)
	})
}

func (s *ForecastService) Get(ctx context.Context, actor model.Partner, id string) (model.ExpenseForecast, error) {
	var out model.ExpenseForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		f, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, f.Scope(), f.PartnerID()); err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}

func (s *ForecastService) Dashboard(ctx context.Context, actor model.Partner) (DashboardView, error) {
	var view DashboardView
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		var open *model.SubmissionWindow
		for i := range all {
			if all[i].State() == model.WindowOpen {
				open = &all[i]
			}
			if all[i].State() == model.WindowClosed {
				view.ClosedYears = append(view.ClosedYears, all[i].Year())
			}
		}
		if open == nil {
			return nil // no open year; ClosedYears still populated
		}
		view.Year = open.Year()
		view.Deadline = open.Deadline()

		forecasts, err := r.Forecasts.ListByYear(ctx, open.Year())
		if err != nil {
			return err
		}
		var authCommon bool
		authSection := map[string]bool{}
		if actor.BoardMember() {
			auths, err := r.BoardAuth.ListByPartner(ctx, actor.ID())
			if err != nil {
				return err
			}
			for _, a := range auths {
				if a.ScopeKind() == model.ScopeCommon {
					authCommon = true
				}
				if a.ScopeKind() == model.ScopeSection {
					authSection[a.SectionCode()] = true
				}
			}
		}
		for _, f := range forecasts {
			switch f.Scope().Kind() {
			case model.ScopePartner:
				if f.PartnerID() == actor.ID() {
					view.Mine = append(view.Mine, f)
				}
			case model.ScopeCommon:
				if authCommon {
					view.BoardScoped = append(view.BoardScoped, f)
				}
			case model.ScopeSection:
				if authSection[f.Scope().SectionCode()] {
					view.BoardScoped = append(view.BoardScoped, f)
				}
			}
		}
		return nil
	})
	return view, err
}

// forecastAudit records a forecast mutation with the soci as actor.
func forecastAudit(ctx context.Context, r ports.RepoSet, actor model.Partner, kind model.AuditKind, forecastID string, at time.Time) error {
	actorID := actor.ID()
	e, err := model.NewAuditEvent(0, &actorID, actor.Email(), kind, "ExpenseForecast", forecastID, at, nil)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}

var _ = strconv.Itoa // keep strconv if unused elsewhere; remove if not needed
```
Notes for the implementer:
- `RepoSet` currently has no `BoardAuth` field and `ForecastRepository` has no `Delete`. **Both must be added** (Tasks below within this task):
  - Add `BoardAuth BoardAuthorizationRepository` to `ports.RepoSet` and wire it in `persistence/txmanager.go` (`NewBoardAuthorizationRepository(q)`).
  - Add `Delete(ctx, id string) error` to `ports.ForecastRepository`, a `DeleteForecast` sqlc query (`DELETE FROM expense_forecast WHERE id = ?`), and the repo method.
  Do these first (they are prerequisites), then the service compiles.
- Remove the trailing `var _ = strconv.Itoa` and the `strconv` import if unused.

- [ ] **Step 5: Add the prerequisites (RepoSet.BoardAuth, Forecasts.Delete)**

1. `internal/domain/ports/tx.go`: add `BoardAuth BoardAuthorizationRepository` to `RepoSet`.
2. `internal/adapters/persistence/txmanager.go`: in the `RepoSet` literal add `BoardAuth: NewBoardAuthorizationRepository(q),`.
3. `internal/domain/ports/ports.go`: add `Delete(ctx context.Context, id string) error` to `ForecastRepository`.
4. `db/queries/forecast.sql`: add
   ```sql
   -- name: DeleteForecast :exec
   DELETE FROM expense_forecast WHERE id = ?;
   ```
   then `make sqlc-generate`.
5. `internal/adapters/persistence/forecast_repository.go`: add
   ```go
   func (r *ForecastRepository) Delete(ctx context.Context, id string) error {
       return r.q.DeleteForecast(ctx, id)
   }
   ```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/application/ -v && go build ./...`
Expected: PASS (all ForecastService cases + existing WindowService tests; ports_check still compiles).

- [ ] **Step 7: Commit**

```bash
git add internal/application/ internal/domain/ports/ internal/adapters/persistence/ db/queries/forecast.sql
git commit -m "feat(application): ForecastService (soci/board CRUD + scope authorization)"
```

---

### Task 4: report.HTMLRenderer

**Files:**
- Create: `internal/adapters/report/htmlrenderer.go`
- Test: `internal/adapters/report/html_test.go`

**Interfaces:**
- Produces: `type HTMLRenderer struct{}`; `(HTMLRenderer) Render(rd report.ReportData) []byte` — HTML built from `buildLayout` (same package, so `buildLayout` is accessible).

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/html_test.go`:
```go
package report

import (
	"strings"
	"testing"
)

func TestHTMLRenderer_StructureAndNumbers(t *testing.T) {
	rd := buildGolden(t)
	html := string(HTMLRenderer{}.Render(rd))

	for _, want := range []string{
		"<h2>Despesa corrent</h2>",
		"<h2>Despesa d&#39;inversió</h2>", // html-escaped apostrophe (template.HTMLEscapeString)
		"<h2>Resum</h2>",
		"<table",
		"2.880,00 €", "23.498,96 €", "11.203,04 €",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}
```
Note: the exact escaping of `'` may be `&#39;` or `&#x27;` depending on the escaper used. If you use `html.EscapeString` (stdlib) it yields `&#39;`. Match the test to the escaper you choose (use `html.EscapeString` for cell/title text).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestHTMLRenderer -v`
Expected: FAIL — undefined `HTMLRenderer`.

- [ ] **Step 3: Write the renderer**

Create `internal/adapters/report/htmlrenderer.go`:
```go
package report

import (
	"fmt"
	"html"
	"strings"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// HTMLRenderer renders ReportData to an HTML fragment using the shared block
// layout, so the on-screen report matches the PDF and Markdown by construction.
type HTMLRenderer struct{}

// Render returns the report as an HTML fragment (a sequence of <h2>/<h3>/<table>).
func (HTMLRenderer) Render(rd report.ReportData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(fmt.Sprintf("Previsions de despeses %d", rd.Year)))
	for _, blk := range buildLayout(rd) {
		switch v := blk.(type) {
		case SectionTitle:
			fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(v.Text))
		case PageBreak:
			// no page concept in HTML
		case Table:
			writeHTMLTable(&b, v)
		}
	}
	return []byte(b.String())
}

func writeHTMLTable(b *strings.Builder, t Table) {
	if t.Title != "" {
		fmt.Fprintf(b, "<h3>%s</h3>\n", html.EscapeString(t.Title))
	}
	b.WriteString("<table>\n")
	if hasNonEmpty(t.Headers) {
		b.WriteString("<thead><tr>")
		for _, h := range t.Headers {
			fmt.Fprintf(b, "<th>%s</th>", html.EscapeString(h))
		}
		b.WriteString("</tr></thead>\n")
	}
	b.WriteString("<tbody>\n")
	for _, row := range t.Rows {
		b.WriteString("<tr")
		if row.Red {
			b.WriteString(` class="red"`)
		}
		b.WriteString(">")
		for _, c := range row.Cells {
			cell := html.EscapeString(c)
			if row.Bold && c != "" {
				cell = "<strong>" + cell + "</strong>"
			}
			fmt.Fprintf(b, "<td>%s</td>", cell)
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
}
```
(`hasNonEmpty` exists in `pdf_doc.go` in the same package — reuse it; if it is lowercase/unexported there, it is accessible. If it does not exist, add it here.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestHTMLRenderer -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/htmlrenderer.go internal/adapters/report/html_test.go
git commit -m "feat(report): HTML report renderer over the shared block layout"
```

---

### Task 5: auth — Authenticator (OAuth/dev) + RequireAuth middleware

**Files:**
- Create: `internal/adapters/auth/middleware.go`
- Create: `internal/adapters/auth/oauth.go`
- Create: `internal/adapters/auth/devlogin.go`
- Test: `internal/adapters/auth/middleware_test.go`

**Interfaces:**
- Consumes: `SessionStore` (Task 2); `ports.PartnerRepository`; `golang.org/x/oauth2`.
- Produces:
  - `type partnerLookup interface { FindByID(ctx, int) (model.Partner, bool, error); FindByEmail(ctx, string) (model.Partner, bool, error) }`
  - context helper: `PartnerFrom(ctx) (model.Partner, bool)`; `withPartner(ctx, p) context.Context`.
  - `RequireAuth(store *SessionStore, partners partnerLookup, next http.Handler) http.Handler` — loads session→partner into context or redirects `/login`.
  - `type Authenticator interface { Login(w, r); Callback(w, r) }` with two impls: `GoogleAuthenticator` (oauth.go) and `DevAuthenticator` (devlogin.go); plus a constructor `NewAuthenticator(cfg, store, partners) (Authenticator, dev bool)` selecting by whether OAuth creds are set.
  - `SetSessionCookie(w, token, secure)` / `ClearSessionCookie(w)`.

- [ ] **Step 1: Add the oauth2 dependency**

Run:
```bash
go get golang.org/x/oauth2@latest
go mod tidy
```

- [ ] **Step 2: Write the failing middleware test**

Create `internal/adapters/auth/middleware_test.go`:
```go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestRequireAuth_RedirectsWhenNoSession(t *testing.T) {
	store, q := newStore(t, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	partners := persistence.NewPartnerRepository(q)
	h := auth.RequireAuth(store, partners, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Errorf("want redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("redirect to %q, want /login", loc)
	}
}

func TestRequireAuth_AttachesPartner(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, q := newStore(t, now) // newStore seeds partner 1 (s1@e.test)
	partners := persistence.NewPartnerRepository(q)
	token, _ := store.Create(context.Background(), 1, "s1@e.test")

	var got model.Partner
	var ok bool
	h := auth.RequireAuth(store, partners, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = auth.PartnerFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "espigol_session", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !ok || got.ID() != 1 {
		t.Errorf("expected authed partner 1; code=%d ok=%v id=%d", rec.Code, ok, got.ID())
	}
}

var _ = sqlc.New
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/auth/ -run TestRequireAuth -v`
Expected: FAIL — undefined `auth.RequireAuth`/`PartnerFrom`.

- [ ] **Step 4: Write middleware.go**

Create `internal/adapters/auth/middleware.go`:
```go
package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

const cookieName = "espigol_session"

type partnerLookup interface {
	FindByID(ctx context.Context, id int) (model.Partner, bool, error)
	FindByEmail(ctx context.Context, email string) (model.Partner, bool, error)
}

type ctxKey int

const partnerKey ctxKey = 0

func withPartner(ctx context.Context, p model.Partner) context.Context {
	return context.WithValue(ctx, partnerKey, p)
}

// PartnerFrom returns the authenticated partner from the request context.
func PartnerFrom(ctx context.Context) (model.Partner, bool) {
	p, ok := ctx.Value(partnerKey).(model.Partner)
	return p, ok
}

// RequireAuth loads the session→partner into the request context or redirects to /login.
func RequireAuth(store *SessionStore, partners partnerLookup, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sess, ok, err := store.Get(r.Context(), c.Value)
		if err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		p, ok, err := partners.FindByID(r.Context(), sess.PartnerID)
		if err != nil || !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(withPartner(r.Context(), p)))
	})
}

// SetSessionCookie writes the session cookie.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(sessionTTL),
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}
```

- [ ] **Step 5: Write the dev + Google authenticators**

Create `internal/adapters/auth/devlogin.go` and `internal/adapters/auth/oauth.go` implementing:
```go
type Authenticator interface {
	// Login handles GET /login (renders form in dev; redirects to Google in prod).
	Login(w http.ResponseWriter, r *http.Request)
	// Complete handles the credential submission: POST /dev-login (dev) or GET /oauth2/callback (prod).
	// On success it creates a session, sets the cookie, and redirects to "/"; on an unregistered
	// email it redirects to /access-denied.
	Complete(w http.ResponseWriter, r *http.Request)
	// IsDev reports whether this is the dev authenticator (controls route registration).
	IsDev() bool
}
```
- `DevAuthenticator{store, partners, secure, loginTmpl}`: `Login` renders the dev email form (a minimal embedded template with the CSRF-free email field — dev only); `Complete` reads `email` from the POST form, `partners.FindByEmail`, on hit `store.Create` + `SetSessionCookie` + redirect `/`, on miss redirect `/access-denied`.
- `GoogleAuthenticator{store, partners, secure, oauthCfg *oauth2.Config}`: `Login` sets a random `oauth_state` cookie + redirects to `oauthCfg.AuthCodeURL(state)`; `Complete` verifies the `state` cookie vs query param, `oauthCfg.Exchange(ctx, code)`, fetches `https://www.googleapis.com/oauth2/v2/userinfo` with the token, decodes `{email}`, `partners.FindByEmail`, then session+cookie+redirect or `/access-denied`. The userinfo fetch is behind a small `emailFetcher` interface so tests can fake it.
- `NewAuthenticator(cfg *config.Config, store *SessionStore, partners partnerLookup) Authenticator`: returns `DevAuthenticator` when `cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == ""`, else `GoogleAuthenticator` built with `&oauth2.Config{ClientID, ClientSecret, Endpoint: google.Endpoint, RedirectURL: cfg.OAuth.RedirectURL, Scopes: []string{"openid","email"}}`. `secure = !dev`.

(Write the concrete code; keep the dev form template inline as a `const` HTML string or a tiny embedded template. The web package registers `/login`→`Login`, `/oauth2/callback`→`Complete` (prod), `/dev-login`→`Complete` (dev) — see Task 6.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/auth/... -v && go build ./...`
Expected: PASS (session + middleware tests; authenticators compile).

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/auth/ go.mod go.sum
git commit -m "feat(auth): RequireAuth middleware + Google/dev authenticators"
```

---

### Task 6: web — server, routes, handlers, templates, CSRF

**Files:**
- Create: `internal/adapters/web/server.go` (replace the stub)
- Create: `internal/adapters/web/handlers.go`
- Create: `internal/adapters/web/templates.go` + `internal/adapters/web/templates/*.html` + `internal/adapters/web/static/espigol.css`
- Create: `internal/adapters/web/csrf.go`
- Test: `internal/adapters/web/server_test.go`

**Interfaces:**
- Consumes: `application.ForecastService`, `auth` (Authenticator, RequireAuth, SessionStore, PartnerFrom), `report.HTMLRenderer`, a `reportReader` (`FindLatestByYear`), `config`.
- Produces: `type Deps struct { Forecasts *application.ForecastService; Auth auth.Authenticator; Sessions *auth.SessionStore; Partners auth.PartnerLookup; Reports reportReader; HTML report.HTMLRenderer; Taxonomy taxonomyReader; Cfg *config.Config; Secure bool }`; `NewServer(Deps) *Server`; `(*Server) Handler() http.Handler`; `(*Server) Run(ctx) error`.

This task is large; build it in this order, each with a focused test in `server_test.go` (all via the **dev-login path**, no Google):

- [ ] **Step 1: Templates + CSS (embedded)**

Create the Catalan templates under `internal/adapters/web/templates/` (`base.html`, `login.html`, `dashboard.html`, `forecast_form.html`, `report.html`, `access_denied.html`, `error.html`) and `internal/adapters/web/static/espigol.css`. `templates.go` parses them with `html/template` via `//go:embed templates/*.html` and exposes a `render(w, name, data)` helper; static CSS served from `//go:embed static`. Keep templates minimal but valid: base layout with the business name + a logout form; dashboard listing `Mine` (CP, concept, brut, edit/delete buttons), a "Nova previsió" link, board `BoardScoped` section when non-empty, and links to `ClosedYears` reports; `forecast_form` with concept/description/amount/planned-date/subtype fields (+ scope selector only when the actor is a board member) and a hidden CSRF field; `report` embeds `{{.ReportHTML}}` (a `template.HTML` value from `HTMLRenderer`).

- [ ] **Step 2: CSRF helper (csrf.go)**

A per-session CSRF token: derive it deterministically from the session token (e.g. HMAC/sha256 of the session token with a process key, hex-encoded) so it needs no extra storage; `csrfToken(sessionToken) string`; `verifyCSRF(r, sessionToken) bool` (compares the `csrf` form field). Templates render `csrfToken` as a hidden field; POST handlers (except dev-login) call `verifyCSRF`.

- [ ] **Step 3: Handlers + routing (server.go, handlers.go)**

`NewServer(deps)` builds a `*http.ServeMux`:
```
GET  /health                 -> 200 OK
GET  /login                  -> deps.Auth.Login
GET  /oauth2/callback        -> deps.Auth.Complete            (registered always; harmless in dev)
POST /dev-login              -> deps.Auth.Complete            (registered only if deps.Auth.IsDev())
POST /logout                 -> clear session + cookie -> /
GET  /access-denied          -> access_denied page
GET  /css/                   -> embedded static
// authed (wrapped in auth.RequireAuth):
GET  /                       -> dashboard (Forecasts.Dashboard)
GET  /forecasts/new          -> forecast_form (new)
POST /forecasts              -> Forecasts.Create
GET  /forecasts/{id}/edit    -> forecast_form (Forecasts.Get)
POST /forecasts/{id}         -> Forecasts.Update
POST /forecasts/{id}/delete  -> Forecasts.Delete
GET  /reports/{year}         -> Reports.FindLatestByYear -> SnapshotFromJSON -> HTML.Render -> report page
```
Handlers read the `Partner` via `auth.PartnerFrom`, parse the form into a `ForecastInput` (parse amount via `model.MoneyFromString`, date via `2006-01-02`, scope from the form for board members else `PARTNER`), and map typed errors: `ErrForbidden`→403 page, `ErrWindowNotOpen`→409 Catalan notice, `ErrForecastNotFound`→404, validation/parse errors→re-render the form with a message. POST success → `http.Redirect(w, r, "/", http.StatusSeeOther)`. `RequireAuth` wraps the authed group; `/reports/{year}` deserializes via `application.SnapshotFromJSON` (web may depend on application).

- [ ] **Step 4: Write the integration test (server_test.go)**

Create `internal/adapters/web/server_test.go` using `httptest` and the dev-login path (no OAuth creds → dev authenticator). Seed a temp SQLite (via the persistence repos: an OPEN 2026 window + taxonomy + a partner + a CLOSED 2025 window with a Report). Build `Deps` from real components (`db.Open`, `ForecastService`, `SessionStore`, `NewAuthenticator` with empty OAuth creds, `report.HTMLRenderer`). Assert:
  - `GET /` unauthenticated → 303 to `/login`.
  - `POST /dev-login` with a registered email → 303 to `/`, sets `espigol_session` cookie.
  - `POST /dev-login` with an unknown email → 303 to `/access-denied`.
  - authed `GET /` → 200, body contains the open year + "Nova previsió".
  - create via `POST /forecasts` (with the CSRF token read from the new-forecast form) → 303; the forecast appears on the dashboard.
  - `POST /forecasts` without a valid CSRF token → rejected (4xx).
  - `GET /reports/2025` (closed) → 200, contains an EU-formatted amount.
  - `POST /logout` → clears the cookie; subsequent `GET /` → redirect to `/login`.

Write these as sub-tests; use an `http.Client` with a cookie jar and redirect-following disabled where you need to inspect 303s.

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/adapters/web/... -v && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/web/
git commit -m "feat(web): socis server — routes, handlers, templates, CSRF"
```

---

### Task 7: wiring (internal/wire) + cmd --server

**Files:**
- Create: `internal/wire/wire.go`
- Modify: `cmd/espigol/main.go`
- Modify: `internal/adapters/web/server.go` (ensure `Run(ctx)` exists; drop the old package-level `Run(ctx,cfg)` stub if still present)
- Test: `internal/wire/wire_test.go`

**Interfaces:**
- Produces: `wire.Server(cfg *config.Config) (*web.Server, error)` — opens the DB, builds repos/services/auth/renderer, returns a ready `*web.Server`.

- [ ] **Step 1: Write the failing test**

Create `internal/wire/wire_test.go`:
```go
package wire_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/wire"
)

func TestServer_AssemblesAndServesHealth(t *testing.T) {
	cfg := &config.Config{DBPath: filepath.Join(t.TempDir(), "wire.db")}
	cfg.Server.Port = 0
	srv, err := wire.Server(cfg)
	if err != nil {
		t.Fatalf("wire.Server: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("health = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wire/ -v`
Expected: FAIL — undefined `wire.Server`.

- [ ] **Step 3: Write wire.go**

Create `internal/wire/wire.go`:
```go
// Package wire assembles the socis web server for `espigol --server`.
package wire

import (
	"fmt"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	reportadapter "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/adapters/system"
	"github.com/pjover/espigol/internal/adapters/web"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
)

// Server opens the database and assembles the socis web server.
func Server(cfg *config.Config) (*web.Server, error) {
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	q := sqlc.New(conn)
	clock := system.SystemClock{}

	partners := persistence.NewPartnerRepository(q)
	reports := persistence.NewReportRepository(q)
	taxonomy := persistence.NewTaxonomyRepository(q)
	txm := persistence.NewTxManager(conn)

	sessions := auth.NewSessionStore(q, clock)
	authn := auth.NewAuthenticator(cfg, sessions, partners)
	forecasts := application.NewForecastService(txm, clock)

	deps := web.Deps{
		Forecasts: forecasts,
		Auth:      authn,
		Sessions:  sessions,
		Partners:  partners,
		Reports:   reports,
		HTML:      reportadapter.HTMLRenderer{},
		Taxonomy:  taxonomy,
		Cfg:       cfg,
		Secure:    !authn.IsDev(),
	}
	return web.NewServer(deps), nil
}
```
(Adjust `web.Deps` field set to match Task 6's actual struct; the dashboard/form needs the OPEN year's taxonomy for the subtype dropdown — expose a `taxonomyReader` via `Taxonomy`. Reconcile names so it compiles.)

- [ ] **Step 4: Wire cmd/espigol --server**

In `cmd/espigol/main.go`, replace the `ModeServer` branch body:
```go
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
```
Update imports: drop `internal/adapters/web` if no longer referenced directly, add `internal/wire`. (Keep `tui` for the default branch.)

- [ ] **Step 5: Run tests + full suite + build the binary**

Run:
```bash
go test ./... && go vet ./... && go build -o bin/espigol ./cmd/espigol
```
Expected: all green; binary builds.

- [ ] **Step 6: Smoke-test the server (dev mode)**

Run:
```bash
ESPIGOL_HOME=$(mktemp -d) ./bin/espigol --server &
sleep 1
curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/health   # expect 200
curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" localhost:8080/   # expect 303 -> /login
kill %1
```
Expected: `/health` → 200; `/` → 303 to `/login` (dev mode, no OAuth creds). Report the output.

- [ ] **Step 7: Commit**

```bash
git add internal/wire/ cmd/espigol/main.go internal/adapters/web/server.go
git commit -m "feat: wire socis server into espigol --server"
```

---

## Self-Review

**Spec coverage:**
- §1 config OAuth.RedirectURL → Task 1.
- §3.1 session table + store → Task 2. §3.2 OAuth/dev flows → Task 5. §3.3 middleware + role → Task 5.
- §4 ForecastService (CRUD + authz, OPEN-window gating, board scope) → Task 3 (+ prerequisite RepoSet.BoardAuth & Forecasts.Delete).
- §5.1 routes / §5.2 handlers+templates+CSRF → Task 6. §5.3 HTMLRenderer → Task 4.
- §6.1 wiring + cmd → Task 7. §6.2 tests (SessionStore, ForecastService, web via dev-login, report view) → Tasks 2,3,6. §6.3 scope (socis-only; no WindowService) → respected.

**Placeholder scan:** Task 5 step 5 and Task 6 describe the authenticators/handlers/templates at a high level with exact route/behavior contracts rather than full literal code (the templates + OAuth glue are large and conventional); every other step has complete code. The Task 3 `var _ = strconv.Itoa` and the `SetNow` test note are explicitly flagged for removal/replacement. These are deliberate, bounded build instructions, not silent gaps — implementers have exact signatures, routes, and test assertions to satisfy.

**Type consistency:** `ForecastInput`/`DashboardView`/`ForecastService` (Task 3) are consumed by web handlers (Task 6) and wire (Task 7). `auth.SessionStore`/`RequireAuth`/`PartnerFrom`/`Authenticator`/`NewAuthenticator`/`SetSessionCookie` (Tasks 2,5) are consumed by web (Task 6) and wire (Task 7). `report.HTMLRenderer` (Task 4) consumed by web/wire. `web.Deps`/`NewServer`/`Server.Handler`/`Run` (Task 6) consumed by wire (Task 7) and cmd. New `ports.RepoSet.BoardAuth` + `ports.ForecastRepository.Delete` (Task 3) are wired in the TxManager. All build on the verified Phase-2…5 signatures in Global Constraints.

**Prerequisite note:** Task 3 adds `RepoSet.BoardAuth` and `ForecastRepository.Delete` — these touch `ports` and `persistence` (incl. `ports_check.go` must still pass and the TxManager RepoSet literal updated). Do Task 3 step 5 before the service body.

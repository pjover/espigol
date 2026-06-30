# Phase 7 — TUI (admin) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The admin-only lazygit-style Bubble Tea TUI plus the per-entity admin application services it drives, and the real PDF renderer finally wired into window close.

**Architecture:** New per-entity application services (Partner/Section/Taxonomy/BoardAuth/Report) + admin methods on ForecastService, all over `ports.TxManager` and audited as the administrator. A modular `Panel`-interface Bubble Tea adapter (`internal/adapters/tui`) drives them; `internal/wire.TUI` assembles everything with the real `report.PDFRenderer`.

**Tech Stack:** Go 1.26, `bubbletea`+`lipgloss` (present) + `bubbles` (new), `modernc.org/sqlite`, sqlc, stdlib.

## Global Constraints

- **Module:** `github.com/pjover/espigol`. Go 1.26. CGO-free. No `float64` in money paths.
- **Admin-only TUI, no auth.** Actor recorded as `cfg.Admin.Email`. The TUI is a driving adapter: depends on `application` + `report` + `config` + Charm libs + stdlib; never imports repositories or `database/sql`.
- **Layering:** `application` → `ports`/`model` + stdlib only. `report` adapter → domain + stdlib. CGO-free throughout.
- **All user-facing text Catalan.** Internal errors English.
- **Existing signatures (verbatim — build on these):**
  - `ports.TxManager.WithinTx(ctx, func(ports.RepoSet) error) error`; `RepoSet{Partners, Forecasts, Windows, Taxonomy, Sections, Reports, Audit, BoardAuth}`.
  - `PartnerRepository`: `Save(ctx,Partner)`, `FindByID(ctx,int)(Partner,bool,error)`, `FindByEmail(ctx,string)(Partner,bool,error)`, `List(ctx)`.
  - `SectionRepository`: `Save`, `List`, `AddMembership(ctx,PartnerSection)`, `ListMembershipsByPartner(ctx,int)`, `ListMemberships(ctx)`.
  - `TaxonomyRepository`: `SaveType`, `SaveSubtype`, `ListTypes(ctx,year)`, `ListSubtypes(ctx,year)`.
  - `WindowRepository`: `Save`, `FindByYear(ctx,year)(SubmissionWindow,bool,error)`, `List`.
  - `BoardAuthorizationRepository`: `Save(ctx,BoardAuthorization)`, `ListByPartner(ctx,int)`.
  - `ForecastRepository`: `Create`, `Save`, `FindByID`, `ListByYear`, `Delete(ctx,id)`.
  - `ReportRepository`: `Insert`, `FindLatestByYear(ctx,year)(Report,bool,error)`, `MarkSuperseded`.
  - Model constructors: `NewPartner(id int, name, surname, vatCode, email, mobile string, pt PartnerType, riaNumber int, addedOn time.Time, boardMember bool)(Partner,error)`; `Partner.WithBoardMember(bool)`; `NewSection(code, label string, active bool, displayOrder int)(Section,error)`; `NewPartnerSection(partnerID int, sectionCode string)(PartnerSection,error)`; `NewExpenseType(year int, code, label string, cat ExpenseCategory)(ExpenseType,error)`; `NewExpenseSubtype(year int, code, label, typeCode string)(ExpenseSubtype,error)`; `NewBoardAuthorization(partnerID int, scopeKind ScopeKind, sectionCode string)(BoardAuthorization,error)`.
  - Enums: `Productor`/`Patrocinador`/`Collaborador` (`PartnerType`); `CategoryCurrent`/`CategoryInvestment`; `ScopeCommon`/`ScopeSection`/`ScopePartner`.
  - `model.NewAuditEvent(id int, actorID *int, actorEmail string, kind AuditKind, entityType, entityID string, ts time.Time, payload *string)`; `model.AuditForecastCreated/Edited/Deleted`. (Audit kinds for entity admin CRUD: reuse existing kinds or add new ones in `model/enums.go` if none fit — see Task 2.)
  - `application.ForecastService`: `NewForecastService(tx ports.TxManager, clock ports.Clock)`; `ForecastInput{Concept, Description string; GrossAmount model.Money; PlannedDate time.Time; SubtypeCode string; ScopeKind model.ScopeKind; SectionCode string}`; package-private `openWindow`, `buildScope`, `forecastAudit`. Errors `ErrWindowNotOpen`, `ErrForbidden`, `ErrForecastNotFound`, `ErrNoOpenWindow`.
  - `application.WindowService`: `CreateYear`, `Open`, `Close`, `Amend`; private method `computeReport(ctx, r ports.RepoSet, w SubmissionWindow)`; package funcs `buildSubtypeCategory`, `appendAudit`. Constructed `NewWindowService(tx, renderer ports.ReportRenderer, clock)`.
  - `report.PDFRenderer{}` `Render(rd report.ReportData, generatedAt time.Time)([]byte,error)` (built with business name+logo — confirm its constructor/fields by reading pdf_renderer.go); `report.MarkdownRenderer{}` `Render(rd report.ReportData)[]byte`; `report.ReportExporter` `Export(rep model.Report, outputDir string) error`; `report.NewReportExporter()`.
  - `db.Open(path)(*sql.DB,error)`; `sqlc.New(db)`; `system.SystemClock{}`; `internal/wire.Server(cfg)` (the Phase-6 wiring pattern to mirror).
  - `config.Config` fields incl. `BusinessName`, `LogoPath`, `OutputDir`, `DBPath`.
- **TDD:** failing test first; commit after each green step.

---

### Task 1: Config — Admin.Email

**Files:** Modify `internal/config/config.go`; Test `internal/config/admin_test.go`.
**Interfaces:** Produces `Config.Admin.Email string` (default `"admin@espigol"`, env `ESPIGOL_ADMIN_EMAIL`).

- [ ] **Step 1: Failing test** — `internal/config/admin_test.go`:
```go
package config

import "testing"

func TestLoad_AdminEmail_Default(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil { t.Fatal(err) }
	if cfg.Admin.Email != "admin@espigol" {
		t.Errorf("Admin.Email default = %q", cfg.Admin.Email)
	}
}

func TestLoad_AdminEmail_EnvOverride(t *testing.T) {
	t.Setenv("ESPIGOL_ADMIN_EMAIL", "boss@coop.cat")
	cfg, err := Load(t.TempDir())
	if err != nil { t.Fatal(err) }
	if cfg.Admin.Email != "boss@coop.cat" {
		t.Errorf("Admin.Email = %q", cfg.Admin.Email)
	}
}
```
- [ ] **Step 2: Verify fail** — `go test ./internal/config/ -run TestLoad_AdminEmail -v` → FAIL (undefined field).
- [ ] **Step 3: Implement** — add to `Config`: `Admin struct{ Email string }`; default `v.SetDefault("admin.email", "admin@espigol")`; binding `cfg.Admin.Email = v.GetString("admin.email")`.
- [ ] **Step 4: Verify** — `go test ./internal/config/ -v && go build ./...` → PASS.
- [ ] **Step 5: Commit** — `git add internal/config/ && git commit -m "feat(config): add Admin.Email"`

---

### Task 2: Persistence — admin mutation queries

Adds the repository methods the admin services need but that don't exist yet: taxonomy delete, board-auth remove, and membership replacement. Plus any new audit kinds.

**Files:** `db/queries/taxonomy.sql`, `db/queries/board.sql`, `db/queries/section.sql` (modify); `internal/domain/ports/ports.go` (modify); `internal/adapters/persistence/{taxonomy,board,section}_repository.go` (modify); `internal/domain/model/enums.go` (modify); regenerate sqlc. Test: `internal/adapters/persistence/admin_mutations_test.go`.

**Interfaces — Produces (add to ports + repos):**
- `TaxonomyRepository.DeleteType(ctx, year int, code string) error`, `DeleteSubtype(ctx, year int, code string) error`
- `BoardAuthorizationRepository.Remove(ctx, partnerID int, scopeKind model.ScopeKind, sectionCode string) error`
- `SectionRepository.RemoveMembershipsByPartner(ctx, partnerID int) error`
- audit kinds in `model/enums.go`: `AuditPartnerSaved`, `AuditSectionSaved`, `AuditTaxonomySaved`, `AuditTaxonomyDeleted`, `AuditBoardAuthChanged` (`= "PARTNER_SAVED"` etc.); add them to the `AuditKind` validity switch.

- [ ] **Step 1: Write the queries**

`db/queries/taxonomy.sql` (append):
```sql
-- name: DeleteExpenseType :exec
DELETE FROM expense_type WHERE year = ? AND code = ?;

-- name: DeleteExpenseSubtype :exec
DELETE FROM expense_subtype WHERE year = ? AND code = ?;
```
`db/queries/board.sql` (append; match the actual table/column names — read the existing board queries first):
```sql
-- name: DeleteBoardAuthorization :exec
DELETE FROM board_authorization
WHERE partner_id = ? AND scope_kind = ? AND section_code = ?;
```
`db/queries/section.sql` (append):
```sql
-- name: DeletePartnerSectionsByPartner :exec
DELETE FROM partner_section WHERE partner_id = ?;
```
Run `make sqlc-generate`; read the generated method names/param structs and reconcile.

- [ ] **Step 2: Add audit kinds** — in `internal/domain/model/enums.go` add the five `AuditKind` consts above and include them in the kind-validation `case`.

- [ ] **Step 3: Add port methods** — extend the three interfaces in `ports.go` with the signatures above.

- [ ] **Step 4: Failing test** — `internal/adapters/persistence/admin_mutations_test.go`: open a temp DB (`db.Open`), seed a year's type+subtype, a partner+membership, a board auth; assert `DeleteSubtype`/`DeleteType` remove them (ListSubtypes/ListTypes shrink), `RemoveMembershipsByPartner` clears memberships, `Remove` drops the board auth (ListByPartner shrinks). Reuse existing persistence test helpers.

- [ ] **Step 5: Verify fail** — `go test ./internal/adapters/persistence/ -run AdminMutations -v` → FAIL (undefined methods).

- [ ] **Step 6: Implement repo methods** — each delegates to the generated query (e.g. `r.q.DeleteExpenseType(ctx, sqlc.DeleteExpenseTypeParams{Year: int64(year), Code: code})`; `ScopeKind` passed as `string(scopeKind)`).

- [ ] **Step 7: Verify + build** — `go test ./internal/adapters/persistence/ -v && go build ./...` → PASS. (`ports_check.go` must still compile.)

- [ ] **Step 8: Commit** — `git add db/ internal/domain/ports/ internal/domain/model/enums.go internal/adapters/persistence/ && git commit -m "feat(persistence): admin mutation queries (taxonomy/board delete, membership clear) + audit kinds"`

---

### Task 3: application.PartnerService

**Files:** Create `internal/application/partner_service.go`; modify `internal/application/errors.go`; Test `internal/application/partner_service_test.go`.

**Interfaces — Produces:**
- `type PartnerService struct{...}`; `NewPartnerService(tx ports.TxManager, clock ports.Clock, adminEmail string) *PartnerService`
- `Create(ctx, PartnerInput) (model.Partner, error)`, `Update(ctx, id int, PartnerInput) error`, `SetBoardMember(ctx, id int, board bool) error`, `SetSectionMemberships(ctx, partnerID int, sectionCodes []string) error`, `List(ctx) ([]model.Partner, error)`
- `type PartnerInput struct { ID int; Name, Surname, VatCode, Email, Mobile string; PartnerType model.PartnerType; RiaNumber int; BoardMember bool }`
- errors: `ErrPartnerExists`, `ErrPartnerNotFound`, `ErrEmailTaken`, `ErrInvalidPartnerType`.

- [ ] **Step 1: Errors** — append to `errors.go`: `ErrPartnerExists`, `ErrPartnerNotFound`, `ErrEmailTaken`, `ErrInvalidPartnerType` (and shared `ErrSectionNotFound` used later).

- [ ] **Step 2: Failing test** — `partner_service_test.go` (real SQLite via `persistence.NewTxManager`, like `forecast_service_test.go`): Create a partner → appears in List with the fields; Create with a duplicate id → `ErrPartnerExists`; Create with an email already used → `ErrEmailTaken`; Update changes fields; `SetBoardMember(id,true)` flips the flag; `SetSectionMemberships(id, ["oliva"])` then again with `["ramaderia"]` replaces (membership list reflects only ramaderia); each mutation appends an audit event with actor = the admin email passed to the constructor. Use a seeded section set.

- [ ] **Step 3: Verify fail** — `go test ./internal/application/ -run Partner -v` → FAIL.

- [ ] **Step 4: Implement** — `partner_service.go`:
```go
package application

import (
	"context"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

type PartnerInput struct {
	ID                                       int
	Name, Surname, VatCode, Email, Mobile    string
	PartnerType                              model.PartnerType
	RiaNumber                                int
	BoardMember                              bool
}

type PartnerService struct {
	tx         ports.TxManager
	clock      ports.Clock
	adminEmail string
}

func NewPartnerService(tx ports.TxManager, clock ports.Clock, adminEmail string) *PartnerService {
	return &PartnerService{tx: tx, clock: clock, adminEmail: adminEmail}
}

func (s *PartnerService) Create(ctx context.Context, in PartnerInput) (model.Partner, error) {
	now := s.clock.Now()
	var created model.Partner
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Partners.FindByID(ctx, in.ID); err != nil {
			return err
		} else if ok {
			return ErrPartnerExists
		}
		if in.Email != "" {
			if _, ok, err := r.Partners.FindByEmail(ctx, in.Email); err != nil {
				return err
			} else if ok {
				return ErrEmailTaken
			}
		}
		p, err := model.NewPartner(in.ID, in.Name, in.Surname, in.VatCode, in.Email, in.Mobile,
			in.PartnerType, in.RiaNumber, now, in.BoardMember)
		if err != nil {
			return err
		}
		if err := r.Partners.Save(ctx, p); err != nil {
			return err
		}
		created = p
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerSaved, "Partner", itoa(in.ID), now)
	})
	return created, err
}

func (s *PartnerService) Update(ctx context.Context, id int, in PartnerInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		existing, ok, err := r.Partners.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPartnerNotFound
		}
		// email uniqueness if changed
		if in.Email != "" && in.Email != existing.Email() {
			if _, taken, err := r.Partners.FindByEmail(ctx, in.Email); err != nil {
				return err
			} else if taken {
				return ErrEmailTaken
			}
		}
		p, err := model.NewPartner(id, in.Name, in.Surname, in.VatCode, in.Email, in.Mobile,
			in.PartnerType, in.RiaNumber, existing.AddedOn(), in.BoardMember)
		if err != nil {
			return err
		}
		if err := r.Partners.Save(ctx, p); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerSaved, "Partner", itoa(id), now)
	})
}

func (s *PartnerService) SetBoardMember(ctx context.Context, id int, board bool) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		existing, ok, err := r.Partners.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPartnerNotFound
		}
		if err := r.Partners.Save(ctx, existing.WithBoardMember(board)); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerSaved, "Partner", itoa(id), now)
	})
}

func (s *PartnerService) SetSectionMemberships(ctx context.Context, partnerID int, sectionCodes []string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Partners.FindByID(ctx, partnerID); err != nil {
			return err
		} else if !ok {
			return ErrPartnerNotFound
		}
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		valid := map[string]bool{}
		for _, sec := range sections {
			valid[sec.Code()] = true
		}
		if err := r.Sections.RemoveMembershipsByPartner(ctx, partnerID); err != nil {
			return err
		}
		for _, code := range sectionCodes {
			if !valid[code] {
				return ErrSectionNotFound
			}
			m, err := model.NewPartnerSection(partnerID, code)
			if err != nil {
				return err
			}
			if err := r.Sections.AddMembership(ctx, m); err != nil {
				return err
			}
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerSaved, "Partner", itoa(partnerID), now)
	})
}

func (s *PartnerService) List(ctx context.Context) ([]model.Partner, error) {
	var out []model.Partner
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Partners.List(ctx)
		return err
	})
	return out, err
}
```
Add a shared `internal/application/admin_audit.go` with:
```go
package application

import (
	"context"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

func itoa(i int) string { return strconv.Itoa(i) }

// adminAudit records an admin mutation with the administrator as actor.
func adminAudit(ctx context.Context, r ports.RepoSet, adminEmail string, kind model.AuditKind, entityType, entityID string, at time.Time) error {
	e, err := model.NewAuditEvent(0, nil, adminEmail, kind, entityType, entityID, at, nil)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}
```

- [ ] **Step 5: Verify** — `go test ./internal/application/ -v && go build ./...` → PASS.
- [ ] **Step 6: Commit** — `git add internal/application/ && git commit -m "feat(application): PartnerService (CRUD + memberships, admin-audited)"`

---

### Task 4: application.SectionService

**Files:** Create `internal/application/section_service.go`; Test `internal/application/section_service_test.go`.

**Interfaces — Produces:** `NewSectionService(tx, clock, adminEmail) *SectionService`; `Create(ctx, SectionInput)(model.Section,error)`, `Update(ctx, code string, SectionInput) error`, `List(ctx)([]model.Section,error)`; `type SectionInput struct { Code, Label string; Active bool; DisplayOrder int }`; errors `ErrSectionExists`, `ErrSectionInUse`.

- [ ] **Step 1: Errors** — append `ErrSectionExists`, `ErrSectionInUse`.
- [ ] **Step 2: Failing test** — Create section "vinya" → in List; duplicate code → `ErrSectionExists`; Update changes label/order; updating a section to `Active=false` while a non-closed year has a forecast with that SECTION scope → `ErrSectionInUse`; audit appended.
- [ ] **Step 3: Verify fail.**
- [ ] **Step 4: Implement** — `Create`: reject if `List` already contains the code; build `model.NewSection`; Save; audit `AuditSectionSaved`. `Update`: require the code exists; if turning inactive, scan `r.Forecasts.ListByYear` across non-closed windows (`r.Windows.List` → those not `WindowClosed`) for a `ScopeSection` forecast with that code → `ErrSectionInUse`; else Save + audit.
- [ ] **Step 5: Verify** — `go test ./internal/application/ -v && go build ./...`.
- [ ] **Step 6: Commit** — `git commit -am "feat(application): SectionService"`

---

### Task 5: application.TaxonomyService

**Files:** Create `internal/application/taxonomy_service.go`; Test `internal/application/taxonomy_service_test.go`.

**Interfaces — Produces:** `NewTaxonomyService(tx, clock, adminEmail) *TaxonomyService`; `CreateType/UpdateType(ctx, TypeInput) error`, `DeleteType(ctx, year int, code string) error`, `CreateSubtype/UpdateSubtype(ctx, SubtypeInput) error`, `DeleteSubtype(ctx, year int, code string) error`, `ListTypes(ctx,year)`, `ListSubtypes(ctx,year)`; inputs `TypeInput{Year int; Code, Label string; Category model.ExpenseCategory}`, `SubtypeInput{Year int; Code, Label, TypeCode string}`; errors `ErrTaxonomyLocked`, `ErrTypeInUse`, `ErrSubtypeInUse`, `ErrTypeNotFound`.

- [ ] **Step 1: Errors** — append the four above.
- [ ] **Step 2: Failing test** — with a **DRAFT** year: create type (CURRENT) + subtype referencing it → appear in lists; create subtype with a missing type → `ErrTypeNotFound`; DeleteSubtype removes it; DeleteType blocked while a subtype references it (`ErrTypeInUse`) but allowed once the subtype is gone; DeleteSubtype blocked if a forecast uses it (`ErrSubtypeInUse`). With an **OPEN** year: any create/update/delete → `ErrTaxonomyLocked`. Audit appended on success.
- [ ] **Step 3: Verify fail.**
- [ ] **Step 4: Implement** — every method first loads the year's window (`r.Windows.FindByYear`); if not found → `ErrWindowNotFound`; if `State() != WindowDraft` → `ErrTaxonomyLocked`. Create/Update build via `model.NewExpenseType`/`NewExpenseSubtype` then `SaveType`/`SaveSubtype`; subtype create/update must find its `TypeCode` in `ListTypes` else `ErrTypeNotFound`. `DeleteType`: reject if any subtype in `ListSubtypes` has `TypeCode==code` (`ErrTypeInUse`), else `DeleteType`. `DeleteSubtype`: reject if any forecast in `ListByYear(year)` has `SubtypeCode==code` (`ErrSubtypeInUse`), else `DeleteSubtype`. Audit `AuditTaxonomySaved`/`AuditTaxonomyDeleted`. (Confirm the DRAFT state constant name, e.g. `model.WindowDraft`, by reading enums.go.)
- [ ] **Step 5: Verify** — `go test ./internal/application/ -v && go build ./...`.
- [ ] **Step 6: Commit** — `git commit -am "feat(application): TaxonomyService (DRAFT-gated CRUD)"`

---

### Task 6: application.BoardAuthorizationService

**Files:** Create `internal/application/board_auth_service.go`; Test `internal/application/board_auth_service_test.go`.

**Interfaces — Produces:** `NewBoardAuthorizationService(tx, clock, adminEmail) *BoardAuthorizationService`; `Grant(ctx, partnerID int, scope model.ScopeKind, sectionCode string) error`, `Revoke(ctx, partnerID int, scope model.ScopeKind, sectionCode string) error`, `ListByPartner(ctx, partnerID int)`; errors `ErrNotBoardMember`, `ErrAuthExists`.

- [ ] **Step 1: Errors** — append `ErrNotBoardMember`, `ErrAuthExists` (reuse `ErrSectionNotFound`, `ErrPartnerNotFound`).
- [ ] **Step 2: Failing test** — Grant COMMON to a board partner → appears in ListByPartner; Grant to a non-board partner → `ErrNotBoardMember`; Grant SECTION with an unknown section → `ErrSectionNotFound`; duplicate Grant → `ErrAuthExists`; Revoke removes it; audit appended.
- [ ] **Step 3: Verify fail.**
- [ ] **Step 4: Implement** — `Grant`: load partner (`ErrPartnerNotFound`), require `BoardMember()` (`ErrNotBoardMember`); for `ScopeSection` require the section exists in `r.Sections.List` (`ErrSectionNotFound`); reject if `ListByPartner` already has a matching (kind, sectionCode) (`ErrAuthExists`); `model.NewBoardAuthorization` → `Save` → audit `AuditBoardAuthChanged`. `Revoke`: `r.BoardAuth.Remove(...)` → audit. (COMMON uses `sectionCode=""`.)
- [ ] **Step 5: Verify + Commit** — `go test ./internal/application/ -v && go build ./...`; `git commit -am "feat(application): BoardAuthorizationService"`

---

### Task 7: ForecastService — admin methods

**Files:** Modify `internal/application/forecast_service.go`; modify `internal/application/errors.go`; Test `internal/application/forecast_admin_test.go`.

**Interfaces — Produces (on the existing `*ForecastService`):**
- `AdminCreate(ctx, actorEmail string, partnerID int, in ForecastInput) (model.ExpenseForecast, error)`
- `AdminUpdate(ctx, actorEmail string, id string, in ForecastInput) error`
- `AdminDelete(ctx, actorEmail string, id string) error`
- error `ErrWindowNotEditable`.

Admin methods impersonate any partner, allow any scope, **bypass** `authorizeScope`, but require the year's window be `DRAFT` or `OPEN`.

- [ ] **Step 1: Error** — append `ErrWindowNotEditable = errors.New("el termini de l'any no permet editar previsions")`.
- [ ] **Step 2: Failing test** — `forecast_admin_test.go` (seed via the existing forecast test helpers, extended): with a DRAFT year, `AdminCreate(admin, partnerID=5, PARTNER input)` succeeds (forecast owned by 5); with an OPEN year, AdminCreate a COMMON forecast succeeds even though admin isn't a board member; AdminUpdate/AdminDelete on those succeed; with a CLOSED year, AdminCreate/Update/Delete → `ErrWindowNotEditable`; audit actor == the passed admin email. (Add a `seedDraft2026`/`seedClosed2026` variant beside the existing seeds.)
- [ ] **Step 3: Verify fail.**
- [ ] **Step 4: Implement** — add a helper and the three methods:
```go
// editableWindow returns the year's window if it is DRAFT or OPEN, else an error.
func editableWindow(ctx context.Context, r ports.RepoSet, year int) (model.SubmissionWindow, error) {
	w, ok, err := r.Windows.FindByYear(ctx, year)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	if !ok {
		return model.SubmissionWindow{}, ErrWindowNotFound
	}
	if w.State() != model.WindowDraft && w.State() != model.WindowOpen {
		return model.SubmissionWindow{}, ErrWindowNotEditable
	}
	return w, nil
}

func (s *ForecastService) AdminCreate(ctx context.Context, actorEmail string, partnerID int, in ForecastInput) (model.ExpenseForecast, error) {
	now := s.clock.Now()
	var created model.ExpenseForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		// the year comes from the input's planned date? No — from the active editable year.
		// Admin edits target a specific year; derive it from the input or the single editable window.
		// Use the partner's intended year: the forecast carries Year from the window context.
		w, err := adminTargetWindow(ctx, r, in)
		if err != nil {
			return err
		}
		scope, err := buildScope(in)
		if err != nil {
			return err
		}
		f, err := model.NewUnsavedExpenseForecast(partnerID, in.Concept, in.Description,
			in.GrossAmount, model.ZeroMoney(), nil, in.PlannedDate, w.Year(), in.SubtypeCode, scope, now, true)
		if err != nil {
			return err
		}
		saved, err := r.Forecasts.Create(ctx, f)
		if err != nil {
			return err
		}
		created = saved
		return forecastAuditEmail(ctx, r, actorEmail, model.AuditForecastCreated, saved.ID(), now)
	})
	return created, err
}
```
Design note for the implementer: the admin must act on a **specific year**. Add a `Year int` field to `ForecastInput` (defaulted/ignored by the soci methods, which derive year from the open window) OR pass `year int` explicitly to the admin methods. **Chosen approach:** add `year int` parameters — change the admin signatures to `AdminCreate(ctx, actorEmail string, year, partnerID int, in ForecastInput)`, `AdminUpdate(ctx, actorEmail string, id string, in ForecastInput)` (Update derives the year from the existing forecast), `AdminDelete(ctx, actorEmail, id)`. Replace `adminTargetWindow` with `editableWindow(ctx, r, year)`. For Update/Delete: load the forecast (`ErrForecastNotFound`), then `editableWindow(ctx, r, existing.Year())`; Update rebuilds via `model.NewExpenseForecast(id, partnerID(existing), ...new fields..., existing.Year(), ...)`; Delete via `r.Forecasts.Delete`. Audit each with `forecastAuditEmail`. Add:
```go
// forecastAuditEmail records a forecast mutation with an explicit actor email (admin).
func forecastAuditEmail(ctx context.Context, r ports.RepoSet, actorEmail string, kind model.AuditKind, id string, at time.Time) error {
	e, err := model.NewAuditEvent(0, nil, actorEmail, kind, "ExpenseForecast", id, at, nil)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}
```
Update the failing test (Step 2) to the final `year`-parameter signatures.

- [ ] **Step 5: Verify** — `go test ./internal/application/ -v && go build ./...` → PASS (soci-facing tests unchanged).
- [ ] **Step 6: Commit** — `git commit -am "feat(application): admin forecast methods (impersonation, DRAFT/OPEN gate)"`

---

### Task 8: application.ReportService (+ shared compute)

**Files:** Modify `internal/application/window_service.go` (extract shared compute); Create `internal/application/report_service.go`; Test `internal/application/report_service_test.go`.

**Interfaces — Produces:**
- package func `computeReportData(ctx context.Context, r ports.RepoSet, w model.SubmissionWindow) (report.ReportData, error)` (the body currently in `WindowService.computeReport`).
- `NewReportService(tx ports.TxManager) *ReportService`; `Preview(ctx, year int) (report.ReportData, error)`; `Latest(ctx, year int) (model.Report, bool, error)`.

- [ ] **Step 1: Refactor (no behavior change)** — move the body of `WindowService.computeReport` into a package function `computeReportData(ctx, r, w)`; make `WindowService.computeReport` call it (or delete it and call the func directly in Close/Amend). Run `go test ./internal/application/ -v` → still green (no new test yet).
- [ ] **Step 2: Failing test** — `report_service_test.go`: seed the anonymized golden 2026 fixture as an **OPEN** year (reuse the Phase-3/5 fixture builder), `Preview(2026)` returns `ReportData` whose totals match the golden numbers (assert a couple, e.g. a category total). For a closed year with a stored Report, `Latest(year)` returns it (`ok==true`); for a year with no report, `ok==false`.
- [ ] **Step 3: Verify fail.**
- [ ] **Step 4: Implement** — `report_service.go`:
```go
package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

type ReportService struct{ tx ports.TxManager }

func NewReportService(tx ports.TxManager) *ReportService { return &ReportService{tx: tx} }

// Preview computes the live allocation for a DRAFT/OPEN year (nothing stored).
func (s *ReportService) Preview(ctx context.Context, year int) (report.ReportData, error) {
	var rd report.ReportData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		rd, err = computeReportData(ctx, r, w)
		return err
	})
	return rd, err
}

// Latest returns the most recent stored Report snapshot for a year.
func (s *ReportService) Latest(ctx context.Context, year int) (model.Report, bool, error) {
	var rep model.Report
	var found bool
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		rep, found, err = r.Reports.FindLatestByYear(ctx, year)
		return err
	})
	return rep, found, err
}
```
- [ ] **Step 5: Verify + Commit** — `go test ./internal/application/ -v && go build ./...`; `git commit -am "feat(application): ReportService (live preview + latest stored) via shared compute"`

---

### Task 9: report.ExportData (live preview export)

**Files:** Modify `internal/adapters/report/exporter.go`; Test `internal/adapters/report/exporter_test.go` (modify existing if present).

**Interfaces — Produces:**
- `NewReportExporter(pdf PDFRenderer) ReportExporter` (now holds a configured PDFRenderer + the MarkdownRenderer).
- `(ReportExporter) ExportData(rd report.ReportData, generatedAt time.Time, outputDir string) error` — renders PDF+MD fresh and writes both files (same filenames as `Export`).
- `Export` unchanged (BLOB-based).

- [ ] **Step 1: Failing test** — `exporter_test.go`: build the golden `ReportData` (reuse the report package's `buildGolden`); `NewReportExporter(PDFRenderer{...})` (construct the PDFRenderer the way pdf_renderer.go requires — read it); `ExportData(rd, fixedTime, tmpDir)`; assert both files exist, `.pdf` starts with `%PDF`, `.md` contains a golden EU number (e.g. `2.880,00 €`). If an existing exporter test constructs `NewReportExporter()` with no args, update it to pass a `PDFRenderer`.
- [ ] **Step 2: Verify fail.**
- [ ] **Step 3: Implement** — change the struct + constructor and add the method:
```go
type ReportExporter struct {
	pdf PDFRenderer
	md  MarkdownRenderer
}

func NewReportExporter(pdf PDFRenderer) ReportExporter { return ReportExporter{pdf: pdf} }

// ExportData renders a live ReportData to PDF + MD files (a preview; not stored).
func (e ReportExporter) ExportData(rd domreport.ReportData, generatedAt time.Time, outputDir string) error {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %q: %w", dir, err)
	}
	base := fmt.Sprintf("Previsions de despeses %d", rd.Year)
	pdfBytes, err := e.pdf.Render(rd, generatedAt)
	if err != nil {
		return fmt.Errorf("rendering pdf: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".pdf"), pdfBytes, 0o644); err != nil {
		return fmt.Errorf("writing pdf: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".md"), e.md.Render(rd), 0o644); err != nil {
		return fmt.Errorf("writing md: %w", err)
	}
	return nil
}
```
(Confirm `report.ReportData` exposes `Year` — used by `buildLayout` already, so it does. Add the `time` import.)
- [ ] **Step 4: Verify** — `go test ./internal/adapters/report/ -v && go build ./...` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "feat(report): ExportData renders a live ReportData preview to files"`

---

### Task 10: TUI foundation — Panel interface, root model, styles, modals

**Files:** Create `internal/adapters/tui/{panel.go,app.go,styles.go,confirm.go,form.go,deps.go}` (replace the Phase-1 `tui.go`); Test `internal/adapters/tui/app_test.go`. Add the `bubbles` dependency.

**Interfaces — Produces:**
- `type Deps struct { Partners *application.PartnerService; Sections *application.SectionService; Taxonomy *application.TaxonomyService; BoardAuth *application.BoardAuthorizationService; Forecasts *application.ForecastService; Windows *application.WindowService; Reports *application.ReportService; Exporter report.ReportExporter; Cfg *config.Config }`
- `type Action struct { Key, Label string }`
- `type Panel interface { Title() string; Update(tea.Msg) (Panel, tea.Cmd); View(width, height int) string; Detail() string; Actions() []Action }`
- `NewApp(deps Deps) *App`; `(*App) Run() error` (wraps `tea.NewProgram(rootModel).Run()`); the root model implements `tea.Model`.
- modal helpers: `confirmModal` (yes/no with a message + an onConfirm `tea.Cmd`), `formModal` (a list of labelled `textinput` fields + optional selectors + onSubmit).

- [ ] **Step 1: Add dependency** — `go get github.com/charmbracelet/bubbles@latest && go mod tidy`.

- [ ] **Step 2: Failing test** — `app_test.go` (headless, no real services needed for nav — use a `Deps{}` with nil services and panels that don't query on construction, OR small fakes): 
  - the root model starts focused on the first panel; a `tea.KeyMsg{Tab}` advances focus to the next panel (assert via an exported test accessor `FocusedTitle()` or by `View()` containing the focused panel's title highlighted);
  - `q` returns a `tea.Quit` command;
  - `?` toggles a help overlay (View contains a keybinding line);
  - opening a `confirmModal` and pressing `y` runs its onConfirm cmd; `n`/`esc` cancels.
  Keep assertions on `Update` return values and `View()` substrings.

- [ ] **Step 3: Verify fail** — `go test ./internal/adapters/tui/ -v` → FAIL.

- [ ] **Step 4: Implement the foundation** — `panel.go` (the interface + `Action`), `styles.go` (lipgloss styles: `stateStyle(state model.WindowState) lipgloss.Style` → DRAFT grey/yellow, OPEN green, CLOSED blue; `dimStyle`, `redStyle`, `titleStyle`, `focusedPanelStyle`, `helpStyle`), `confirm.go` + `form.go` (the modal models), and `app.go`:
  - root model fields: `deps Deps`, `panels []Panel`, `focused int`, `year int` (year context), `help help.Model`, `modal tea.Model` (nil when none), `width,height int`.
  - `Init` returns each panel's initial load cmd (batch).
  - `Update`: if a `modal` is active, route to it (and clear it when it returns a "close" msg); handle `tea.WindowSizeMsg`; global keys `tab`/`shift+tab`/left/right (move focus), `?` (toggle help), `q`/`ctrl+c` (quit); otherwise delegate to `panels[focused].Update(msg)`.
  - `View`: lipgloss layout — top bar (business name + `Any: <year>`), left column of panel titles (focused one styled), main area = focused panel's `View`/`Detail`, bottom = `help`/action line built from `panels[focused].Actions()`; overlay the modal if present.
  - `NewApp`/`Run` as above. Provide a small exported `FocusedTitle() string` test accessor on the model (or test via View substring).

- [ ] **Step 5: Verify** — `go test ./internal/adapters/tui/ -v && go build ./...` → PASS.
- [ ] **Step 6: Commit** — `git add internal/adapters/tui/ go.mod go.sum && git commit -m "feat(tui): foundation — Panel interface, root model, styles, modals"`

---

### Task 11: TUI panels — Anys, Socis, Seccions

**Files:** Create `internal/adapters/tui/panel_{years,partners,sections}.go`; Test `internal/adapters/tui/panel_basic_test.go`.

**Interfaces — Consumes:** `Deps`, `Panel`, modals, styles. Each panel struct embeds a `bubbles/list` (or `table`) and a `Deps`, loads via a `tea.Cmd` that calls the service, and implements the `Panel` interface.

- [ ] **Step 1: Failing tests** — `panel_basic_test.go` with small in-memory **fakes** implementing the service method subsets each panel uses (or real services over a temp DB — pick the lighter: real services over `db.Open` temp DB seeded via `wire`-style helpers is most faithful; use that). Assert per panel:
  - **Anys:** lists windows with state styling; `n` opens the create-year form; `o`/`c` on a selected row open the confirm modal then call `Windows.Open`/`Close`; `r` triggers the report action (Task 12 wires the actual generation — here assert it dispatches the report cmd); after a mutation the list reloads.
  - **Socis:** lists partners; `n`/`e` open the partner form; `d` opens confirm; `b` toggles board via `Partners.SetBoardMember`; `m` opens the memberships form.
  - **Seccions:** lists sections; `n`/`e` open the section form; submit calls `Sections.Create`/`Update` and reloads.
- [ ] **Step 2: Verify fail.**
- [ ] **Step 3: Implement the three panels.** Each: a `list.Model` of items built from the service list; `Update` handles `↑/↓` (delegate to the list), the panel's single-letter actions (open form/confirm modals via messages the root model interprets, or return the modal as a cmd/msg per the foundation's modal protocol), and a custom `panelLoadedMsg`/`mutationDoneMsg` to reload; `View` renders the list with state/dim/red styles; `Detail` shows the selected item's fields; `Actions` returns the key list. Year-context: Anys changing selection updates the root `year` (via a `yearSelectedMsg`).
- [ ] **Step 4: Verify** — `go test ./internal/adapters/tui/ -v && go build ./...`.
- [ ] **Step 5: Commit** — `git commit -am "feat(tui): Anys, Socis, Seccions panels"`

---

### Task 12: TUI panels — Tipus, Previsions, Informes (+ report action)

**Files:** Create `internal/adapters/tui/panel_{taxonomy,forecasts,reports}.go`; Test `internal/adapters/tui/panel_admin_test.go`.

- [ ] **Step 1: Failing tests** — (real services over a temp DB):
  - **Tipus i subtipus:** lists the year-context's types+subtypes; `n/e/d` call `Taxonomy.*`; when the year-context window is **not DRAFT**, the create/edit/delete actions are absent from `Actions()` and a "només editable en esborrany" notice shows; with DRAFT they work.
  - **Previsions:** lists forecasts for the year-context (all partners); `n/e` open a form including a partner selector + scope selector + subtype selector; submit calls `Forecasts.AdminCreate`/`AdminUpdate` with `cfg.Admin.Email` and the year-context; `d` confirms then `AdminDelete`.
  - **Informes:** `r` generates the report for the year-context — assert it calls `Exporter.Export` (when the year is CLOSED, via `Reports.Latest`) or `Exporter.ExportData` (when DRAFT/OPEN, via `Reports.Preview`) and shows the written paths. Use a temp `cfg.OutputDir`; assert the files exist after the action.
- [ ] **Step 2: Verify fail.**
- [ ] **Step 3: Implement the three panels + the report action.** The report action (shared helper `generateReport(deps, year, state) tea.Cmd`): if `state==CLOSED` → `Reports.Latest(year)` → `Exporter.Export(rep, cfg.OutputDir)`; else `Reports.Preview(year)` → `Exporter.ExportData(rd, now, cfg.OutputDir)`; return a `reportDoneMsg{paths, err}` the panel renders. Taxonomy panel gates its `Actions()` on the year-context window state (query on load).
- [ ] **Step 4: Verify** — `go test ./internal/adapters/tui/ -v && go build ./...`.
- [ ] **Step 5: Commit** — `git commit -am "feat(tui): Tipus, Previsions, Informes panels + report generation"`

---

### Task 13: Wiring — wire.TUI + cmd --default + real PDF in Close

**Files:** Modify `internal/wire/wire.go`; modify `cmd/espigol/main.go`; Test `internal/wire/tui_test.go`.

**Interfaces — Produces:** `wire.TUI(cfg *config.Config) (*tui.App, error)`.

- [ ] **Step 1: Failing test** — `internal/wire/tui_test.go`: `cfg` with a temp `DBPath`; `app, err := wire.TUI(cfg)`; assert `err==nil` and `app != nil`. (Assembly smoke; do not start the TTY program.)
- [ ] **Step 2: Verify fail.**
- [ ] **Step 3: Implement `wire.TUI`** — mirror `wire.Server`: `db.Open` → `sqlc.New` → repos + `TxManager` + `system.SystemClock{}`; build `report.PDFRenderer` (business name + logo from cfg — same construction the renderer requires), `report.NewReportExporter(pdfRenderer)`, `application.NewWindowService(txm, pdfRenderer, clock)` (**real renderer, not the no-op**), `NewPartnerService`/`NewSectionService`/`NewTaxonomyService`/`NewBoardAuthorizationService(txm, clock, cfg.Admin.Email)`, `NewForecastService(txm, clock)`, `NewReportService(txm)`; assemble `tui.Deps` and `return tui.NewApp(deps), nil`.
- [ ] **Step 4: Wire cmd** — in `cmd/espigol/main.go` `default` branch, replace `tui.Run(cfg)` with:
```go
	default:
		app, err := wire.TUI(cfg)
		if err != nil {
			log.Fatalf("espigol tui: %v", err)
		}
		if err := app.Run(); err != nil {
			log.Fatalf("espigol tui: %v", err)
		}
```
Drop the now-unused direct `tui` import if `wire` is the only entry (keep whatever compiles).
- [ ] **Step 5: Verify full suite + build the binary** — `go test ./... && go vet ./... && CGO_ENABLED=0 go build -o bin/espigol ./cmd/espigol` → all green. (Do NOT commit `bin/`.)
- [ ] **Step 6: Commit** — `git add internal/wire/ cmd/espigol/main.go && git commit -m "feat: wire admin TUI (real PDFRenderer into Close) into espigol default mode"`

---

## Self-Review

**Spec coverage:** §1 config Admin.Email → T1. §3 services → PartnerService T3, SectionService T4, TaxonomyService T5 (DRAFT gate), BoardAuthorizationService T6, ForecastService admin methods T7, ReportService T8 (shared compute); supporting persistence (deletes/membership clear) → T2. §4 TUI (Panel interface, root model, modals, styles, six panels) → T10 (foundation) + T11 + T12. §5 report flow (live preview vs stored export) + real PDFRenderer into Close → T9 (`ExportData`) + T12 (report action) + T13 (Close wiring). §6 testing → per-task TDD + the wire smoke. §7 wiring → T13.

**Placeholder scan:** Application/persistence/report/config/wiring tasks (T1–T9, T13) carry complete code. The TUI tasks (T10–T12) specify the `Panel` interface, `Deps`, modal protocol, per-panel key actions, and per-panel test assertions as exact contracts but not full literal Bubble Tea code for every panel (the rendering/`list` wiring is large and conventional) — deliberate, bounded, and flagged here, consistent with how Phase 6's web tasks were specified.

**Type consistency:** `Deps`, `Panel`, `Action`, `NewApp`/`Run` (T10) are consumed by T11–T13. The per-entity service constructors and method signatures defined in T3–T8 are consumed by the panels (T11–T12) and `wire.TUI` (T13). `report.NewReportExporter(pdf)` + `ExportData` (T9) consumed by T12/T13. New ports methods + audit kinds (T2) consumed by T3–T7. `ForecastService.Admin*` final signatures take an explicit `year` (T7) — the Previsions panel (T12) passes the year-context. `editableWindow`/`computeReportData` (T7/T8) are package-private to `application`.

**Prerequisite note:** T2 (persistence + audit kinds) must land before T3–T7; T7/T8 before the panels that call them; T9 before T12/T13; T10 before T11/T12; T13 last. Confirm `model.WindowDraft` (and `WindowState` accessor) names by reading `enums.go` during T5/T7.

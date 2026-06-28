# Phase 2 — Domain & Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the immutable domain model and the SQLite persistence layer (goose schema, sqlc queries, mappers, repositories) on the Phase 1 foundation, plus a standalone `cmd/adopt` tool that transforms the inherited Java SQLite database into the new schema.

**Architecture:** Hexagonal. `internal/domain/model` holds immutable value structs (unexported fields, validating constructors, `With…` copy methods) with zero adapter imports. `internal/domain/ports` holds repository interfaces. `internal/adapters/persistence` holds goose migrations (the schema), sqlc-generated DBOs, mappers, and repositories implementing the ports. `cmd/adopt` is a one-off binary with its own legacy-schema reader.

**Tech Stack:** Go 1.26, `github.com/shopspring/decimal`, `modernc.org/sqlite` (pure Go), `github.com/pressly/goose/v3`, `sqlc` (generated code, no runtime dep), `database/sql`.

## Global Constraints

- **Module path:** `github.com/pjover/espigol`. Go **1.26**. CGO-free (pure-Go sqlite only — never add `mattn/go-sqlite3`).
- **No `float64` in the domain or persistence.** Money is `shopspring/decimal` (scale 2, HALF_UP) and is stored as **TEXT** (e.g. `"31900.00"`).
- **The `domain` package imports nothing from `adapters/`, and nothing from `database/sql`, `driver`, or `sqlc`.** Serialization lives in the mapper.
- **All user-facing text is Catalan.** Enum *values* persisted/displayed in Catalan where the Java code used Catalan (`PartnerType` → "Productor" etc.). Scope is persisted as the English kind (`COMMON`/`SECTION`/`PARTNER`); its Catalan display derives from the `Section` label later.
- **Money in SQLite = TEXT**; **timestamps = RFC3339 UTC** (`time.RFC3339`, e.g. `2026-03-01T00:00:00Z`); **dates = `2006-01-02`**.
- **`CPYYnnn` ids:** `CP` + 2-digit year + 3-char sequence; sequence `0–999` as `%03d`, `1000–3599` as `letter('A'+(m/100))` + `%02d(m%100)` where `m=seq-1000`.
- **sqlc schema source = the goose migrations** (`db/migrations/`). Regenerate with `make sqlc-generate` after changing queries or schema.
- **TDD:** every behavioral change starts with a failing test. Commit after each green step.
- **Shared signatures** (defined in the tasks below; listed here so every task uses identical names):
  - `model.Money` — `MoneyFromString(string) (Money, error)`, `MoneyOf(int64) Money`, `ZeroMoney() Money`; methods `Plus`, `Minus`, `Times(int) Money`, `TimesRatio(decimal.Decimal) Money`, `DividedBy(int) Money`, `Negate() Money`, `Cmp(Money) int`, `IsZero() bool`, `Decimal() decimal.Decimal`, `String() string`.
  - `model.ExpenseScope` — `NewCommonScope()`, `NewPartnerScope()`, `NewSectionScope(code string) (ExpenseScope, error)`, `NewScope(kind ScopeKind, sectionCode string) (ExpenseScope, error)`; accessors `Kind() ScopeKind`, `SectionCode() string`.
  - `forecastid.Format(year, seq int) (string, error)`, `forecastid.ParseSeq(id string) (year, seq int, err error)`.

---

### Task 1: Money value type

**Files:**
- Create: `internal/domain/model/money.go`
- Test: `internal/domain/model/money_test.go`

**Interfaces:**
- Consumes: `github.com/shopspring/decimal`.
- Produces: `model.Money` with the methods listed in Global Constraints.

- [ ] **Step 1: Add the decimal dependency**

Run:
```bash
go get github.com/shopspring/decimal@latest
```
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the failing test**

Create `internal/domain/model/money_test.go`:
```go
package model

import "testing"

func TestMoneyFromString_NormalizesScale(t *testing.T) {
	m, err := MoneyFromString("31900")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.String() != "31900.00" {
		t.Errorf("String() = %q, want %q", m.String(), "31900.00")
	}
}

func TestMoneyFromString_Rejects(t *testing.T) {
	if _, err := MoneyFromString("not-a-number"); err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestMoneyArithmetic(t *testing.T) {
	a, _ := MoneyFromString("10.00")
	b, _ := MoneyFromString("3.00")
	if got := a.Plus(b).String(); got != "13.00" {
		t.Errorf("Plus = %q, want 13.00", got)
	}
	if got := a.Minus(b).String(); got != "7.00" {
		t.Errorf("Minus = %q, want 7.00", got)
	}
	if got := b.Times(3).String(); got != "9.00" {
		t.Errorf("Times = %q, want 9.00", got)
	}
	if got := a.DividedBy(3).String(); got != "3.33" {
		t.Errorf("DividedBy = %q, want 3.33 (HALF_UP scale 2)", got)
	}
	if got := a.Negate().String(); got != "-10.00" {
		t.Errorf("Negate = %q, want -10.00", got)
	}
}

func TestMoneyCmpAndZero(t *testing.T) {
	a, _ := MoneyFromString("10.00")
	b, _ := MoneyFromString("3.00")
	if a.Cmp(b) <= 0 {
		t.Errorf("expected a > b")
	}
	if !ZeroMoney().IsZero() {
		t.Errorf("ZeroMoney should be zero")
	}
	if MoneyOf(5).String() != "5.00" {
		t.Errorf("MoneyOf(5) = %q, want 5.00", MoneyOf(5).String())
	}
}

func TestMoney_RealValueRoundTrips(t *testing.T) {
	// The former-REAL legacy value must survive exactly.
	m, err := MoneyFromString("1322.22")
	if err != nil {
		t.Fatal(err)
	}
	if m.String() != "1322.22" {
		t.Errorf("String() = %q, want 1322.22", m.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/domain/model/ -run TestMoney -v`
Expected: FAIL — `undefined: MoneyFromString`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/domain/model/money.go`:
```go
package model

import (
	"github.com/shopspring/decimal"
)

// Money is an immutable monetary amount, always scale 2, HALF_UP rounding.
// It deliberately imports no database/sql types; serialization lives in the
// persistence mapper via String() and MoneyFromString.
type Money struct {
	amount decimal.Decimal
}

func normalize(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

// MoneyFromString parses a decimal string (e.g. "31900.00") into Money.
func MoneyFromString(s string) (Money, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: normalize(d)}, nil
}

// MoneyOf builds Money from a whole-unit integer amount.
func MoneyOf(value int64) Money {
	return Money{amount: normalize(decimal.NewFromInt(value))}
}

// ZeroMoney returns a zero amount.
func ZeroMoney() Money {
	return Money{amount: normalize(decimal.Zero)}
}

func (m Money) Plus(other Money) Money  { return Money{amount: normalize(m.amount.Add(other.amount))} }
func (m Money) Minus(other Money) Money { return Money{amount: normalize(m.amount.Sub(other.amount))} }
func (m Money) Times(factor int) Money {
	return Money{amount: normalize(m.amount.Mul(decimal.NewFromInt(int64(factor))))}
}
func (m Money) TimesRatio(ratio decimal.Decimal) Money {
	return Money{amount: normalize(m.amount.Mul(ratio))}
}

// DividedBy divides by a positive integer (HALF_UP, scale 2). Panics on divisor < 1,
// mirroring the reference implementation's IllegalArgumentException.
func (m Money) DividedBy(divisor int) Money {
	if divisor < 1 {
		panic("Money.DividedBy: divisor must be >= 1")
	}
	return Money{amount: m.amount.DivRound(decimal.NewFromInt(int64(divisor)), 2)}
}

func (m Money) Negate() Money              { return Money{amount: normalize(m.amount.Neg())} }
func (m Money) Cmp(other Money) int        { return m.amount.Cmp(other.amount) }
func (m Money) IsZero() bool               { return m.amount.IsZero() }
func (m Money) Decimal() decimal.Decimal   { return m.amount }

// String returns the canonical fixed-scale form, e.g. "31900.00".
func (m Money) String() string { return m.amount.StringFixed(2) }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/domain/model/ -run TestMoney -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
go mod tidy
git add internal/domain/model/money.go internal/domain/model/money_test.go go.mod go.sum
git commit -m "feat(model): add Money value type backed by shopspring/decimal"
```

---

### Task 2: Enums and ExpenseScope

**Files:**
- Create: `internal/domain/model/enums.go`
- Create: `internal/domain/model/scope.go`
- Test: `internal/domain/model/enums_test.go`
- Test: `internal/domain/model/scope_test.go`

**Interfaces:**
- Produces:
  - `ScopeKind` (string) consts `ScopeCommon="COMMON"`, `ScopeSection="SECTION"`, `ScopePartner="PARTNER"`; `ParseScopeKind(string) (ScopeKind, error)`.
  - `WindowState` (string) consts `WindowDraft="DRAFT"`, `WindowOpen="OPEN"`, `WindowClosed="CLOSED"`; `ParseWindowState`.
  - `ExpenseCategory` (string) consts `CategoryCurrent="CURRENT"`, `CategoryInvestment="INVESTMENT"`; `ParseExpenseCategory`.
  - `PartnerType` (string) consts `Productor="Productor"`, `Patrocinador="Patrocinador"`, `Collaborador="Col·laborador"`; `ParsePartnerType`.
  - `AuditKind` (string) consts for `LOGIN, FORECAST_CREATED, FORECAST_EDITED, FORECAST_DELETED, WINDOW_OPENED, WINDOW_CLOSED, WINDOW_AUTO_CLOSED, REPORT_GENERATED, PARTNER_CREATED, PARTNER_EDITED, NOTIFICATION_SENT, MIGRATION`; `ParseAuditKind`.
  - `ExpenseScope` and its constructors/accessors (Global Constraints).

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/model/enums_test.go`:
```go
package model

import "testing"

func TestParsePartnerType(t *testing.T) {
	pt, err := ParsePartnerType("Productor")
	if err != nil || pt != Productor {
		t.Fatalf("got (%v,%v), want Productor", pt, err)
	}
	if _, err := ParsePartnerType("Nope"); err == nil {
		t.Fatal("expected error for unknown partner type")
	}
}

func TestParseWindowState(t *testing.T) {
	if s, err := ParseWindowState("OPEN"); err != nil || s != WindowOpen {
		t.Fatalf("got (%v,%v), want OPEN", s, err)
	}
	if _, err := ParseWindowState("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseExpenseCategory(t *testing.T) {
	if c, err := ParseExpenseCategory("INVESTMENT"); err != nil || c != CategoryInvestment {
		t.Fatalf("got (%v,%v), want INVESTMENT", c, err)
	}
}

func TestParseAuditKind(t *testing.T) {
	if k, err := ParseAuditKind("MIGRATION"); err != nil || k != AuditMigration {
		t.Fatalf("got (%v,%v), want MIGRATION", k, err)
	}
}
```

Create `internal/domain/model/scope_test.go`:
```go
package model

import "testing"

func TestNewSectionScope(t *testing.T) {
	s, err := NewSectionScope("oliva")
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind() != ScopeSection || s.SectionCode() != "oliva" {
		t.Errorf("got kind=%q code=%q", s.Kind(), s.SectionCode())
	}
}

func TestNewSectionScope_RejectsEmpty(t *testing.T) {
	if _, err := NewSectionScope(""); err == nil {
		t.Fatal("expected error for empty section code")
	}
}

func TestCommonAndPartnerScopesHaveNoSection(t *testing.T) {
	if NewCommonScope().SectionCode() != "" || NewCommonScope().Kind() != ScopeCommon {
		t.Error("common scope wrong")
	}
	if NewPartnerScope().SectionCode() != "" || NewPartnerScope().Kind() != ScopePartner {
		t.Error("partner scope wrong")
	}
}

func TestNewScope_InvariantSectionIffSectionKind(t *testing.T) {
	if _, err := NewScope(ScopeCommon, "oliva"); err == nil {
		t.Error("COMMON with a section code must error")
	}
	if _, err := NewScope(ScopeSection, ""); err == nil {
		t.Error("SECTION without a section code must error")
	}
	if _, err := NewScope(ScopeSection, "oliva"); err != nil {
		t.Errorf("SECTION with code must succeed: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/model/ -run 'TestParse|TestNew|TestCommon' -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Write the enums**

Create `internal/domain/model/enums.go`:
```go
package model

import "fmt"

type ScopeKind string

const (
	ScopeCommon  ScopeKind = "COMMON"
	ScopeSection ScopeKind = "SECTION"
	ScopePartner ScopeKind = "PARTNER"
)

func ParseScopeKind(s string) (ScopeKind, error) {
	switch ScopeKind(s) {
	case ScopeCommon, ScopeSection, ScopePartner:
		return ScopeKind(s), nil
	default:
		return "", fmt.Errorf("unknown ScopeKind: %q", s)
	}
}

type WindowState string

const (
	WindowDraft  WindowState = "DRAFT"
	WindowOpen   WindowState = "OPEN"
	WindowClosed WindowState = "CLOSED"
)

func ParseWindowState(s string) (WindowState, error) {
	switch WindowState(s) {
	case WindowDraft, WindowOpen, WindowClosed:
		return WindowState(s), nil
	default:
		return "", fmt.Errorf("unknown WindowState: %q", s)
	}
}

type ExpenseCategory string

const (
	CategoryCurrent    ExpenseCategory = "CURRENT"
	CategoryInvestment ExpenseCategory = "INVESTMENT"
)

func ParseExpenseCategory(s string) (ExpenseCategory, error) {
	switch ExpenseCategory(s) {
	case CategoryCurrent, CategoryInvestment:
		return ExpenseCategory(s), nil
	default:
		return "", fmt.Errorf("unknown ExpenseCategory: %q", s)
	}
}

type PartnerType string

const (
	Productor     PartnerType = "Productor"
	Patrocinador  PartnerType = "Patrocinador"
	Collaborador  PartnerType = "Col·laborador"
)

func ParsePartnerType(s string) (PartnerType, error) {
	switch PartnerType(s) {
	case Productor, Patrocinador, Collaborador:
		return PartnerType(s), nil
	default:
		return "", fmt.Errorf("unknown PartnerType: %q", s)
	}
}

type AuditKind string

const (
	AuditLogin            AuditKind = "LOGIN"
	AuditForecastCreated  AuditKind = "FORECAST_CREATED"
	AuditForecastEdited   AuditKind = "FORECAST_EDITED"
	AuditForecastDeleted  AuditKind = "FORECAST_DELETED"
	AuditWindowOpened     AuditKind = "WINDOW_OPENED"
	AuditWindowClosed     AuditKind = "WINDOW_CLOSED"
	AuditWindowAutoClosed AuditKind = "WINDOW_AUTO_CLOSED"
	AuditReportGenerated  AuditKind = "REPORT_GENERATED"
	AuditPartnerCreated   AuditKind = "PARTNER_CREATED"
	AuditPartnerEdited    AuditKind = "PARTNER_EDITED"
	AuditNotificationSent AuditKind = "NOTIFICATION_SENT"
	AuditMigration        AuditKind = "MIGRATION"
)

func ParseAuditKind(s string) (AuditKind, error) {
	switch AuditKind(s) {
	case AuditLogin, AuditForecastCreated, AuditForecastEdited, AuditForecastDeleted,
		AuditWindowOpened, AuditWindowClosed, AuditWindowAutoClosed, AuditReportGenerated,
		AuditPartnerCreated, AuditPartnerEdited, AuditNotificationSent, AuditMigration:
		return AuditKind(s), nil
	default:
		return "", fmt.Errorf("unknown AuditKind: %q", s)
	}
}
```

- [ ] **Step 4: Write ExpenseScope**

Create `internal/domain/model/scope.go`:
```go
package model

import "fmt"

// ExpenseScope is the scope of a forecast: COMMON, a specific SECTION, or PARTNER.
// sectionCode is set iff kind == ScopeSection.
type ExpenseScope struct {
	kind        ScopeKind
	sectionCode string
}

func NewCommonScope() ExpenseScope  { return ExpenseScope{kind: ScopeCommon} }
func NewPartnerScope() ExpenseScope { return ExpenseScope{kind: ScopePartner} }

func NewSectionScope(code string) (ExpenseScope, error) {
	if code == "" {
		return ExpenseScope{}, fmt.Errorf("section scope requires a non-empty section code")
	}
	return ExpenseScope{kind: ScopeSection, sectionCode: code}, nil
}

// NewScope builds a scope from a kind and an optional section code, enforcing
// that the section code is present iff the kind is SECTION.
func NewScope(kind ScopeKind, sectionCode string) (ExpenseScope, error) {
	switch kind {
	case ScopeSection:
		return NewSectionScope(sectionCode)
	case ScopeCommon, ScopePartner:
		if sectionCode != "" {
			return ExpenseScope{}, fmt.Errorf("scope %s must not carry a section code", kind)
		}
		return ExpenseScope{kind: kind}, nil
	default:
		return ExpenseScope{}, fmt.Errorf("unknown ScopeKind: %q", kind)
	}
}

func (s ExpenseScope) Kind() ScopeKind     { return s.kind }
func (s ExpenseScope) SectionCode() string { return s.sectionCode }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/model/ -v`
Expected: PASS (Money + enums + scope).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/model/enums.go internal/domain/model/scope.go internal/domain/model/enums_test.go internal/domain/model/scope_test.go
git commit -m "feat(model): add enums and ExpenseScope kind+section"
```

---

### Task 3: forecastid helper

**Files:**
- Create: `internal/domain/forecastid/forecastid.go`
- Test: `internal/domain/forecastid/forecastid_test.go`

**Interfaces:**
- Produces: `forecastid.Format(year, seq int) (string, error)`, `forecastid.ParseSeq(id string) (year, seq int, err error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/forecastid/forecastid_test.go`:
```go
package forecastid

import "testing"

func TestFormat_Decimal(t *testing.T) {
	cases := map[int]string{0: "CP26000", 1: "CP26001", 36: "CP26036", 999: "CP26999"}
	for seq, want := range cases {
		got, err := Format(2026, seq)
		if err != nil || got != want {
			t.Errorf("Format(2026,%d) = (%q,%v), want %q", seq, got, err, want)
		}
	}
}

func TestFormat_LetterOverflow(t *testing.T) {
	cases := map[int]string{1000: "CP26A00", 1099: "CP26A99", 1100: "CP26B00", 3599: "CP26Z99"}
	for seq, want := range cases {
		got, err := Format(2026, seq)
		if err != nil || got != want {
			t.Errorf("Format(2026,%d) = (%q,%v), want %q", seq, got, err, want)
		}
	}
}

func TestFormat_OutOfRange(t *testing.T) {
	if _, err := Format(2026, 3600); err == nil {
		t.Error("expected error for seq > 3599")
	}
	if _, err := Format(2026, -1); err == nil {
		t.Error("expected error for negative seq")
	}
}

func TestParseSeq_RoundTrip(t *testing.T) {
	for _, seq := range []int{0, 1, 36, 999, 1000, 1099, 1100, 3599} {
		id, _ := Format(2026, seq)
		y, gotSeq, err := ParseSeq(id)
		if err != nil || y != 2026 || gotSeq != seq {
			t.Errorf("ParseSeq(%q) = (%d,%d,%v), want (2026,%d)", id, y, gotSeq, err, seq)
		}
	}
}

func TestParseSeq_Invalid(t *testing.T) {
	for _, bad := range []string{"", "XX26001", "CP2601", "CP26@00", "CP260000"} {
		if _, _, err := ParseSeq(bad); err == nil {
			t.Errorf("ParseSeq(%q) should error", bad)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/forecastid/ -v`
Expected: FAIL — undefined `Format`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/forecastid/forecastid.go`:
```go
// Package forecastid formats and parses expense-forecast ids of the form
// CP + 2-digit year + 3-char sequence. Sequence 0..999 is decimal (%03d);
// 1000..3599 uses a leading letter for the hundreds block: 1000 -> "A00",
// 1099 -> "A99", 1100 -> "B00", ... 3599 -> "Z99".
package forecastid

import (
	"fmt"
	"strconv"
)

const maxSeq = 3599 // 999 decimal + 26*100 letter-block slots - 1

// Format renders (year, seq) into a CPYYnnn id.
func Format(year, seq int) (string, error) {
	if seq < 0 || seq > maxSeq {
		return "", fmt.Errorf("forecast sequence out of range [0,%d]: %d", maxSeq, seq)
	}
	yy := year % 100
	if seq <= 999 {
		return fmt.Sprintf("CP%02d%03d", yy, seq), nil
	}
	m := seq - 1000
	letter := byte('A' + m/100)
	return fmt.Sprintf("CP%02d%c%02d", yy, letter, m%100), nil
}

// ParseSeq is the inverse of Format. year is reconstructed as 2000+YY.
func ParseSeq(id string) (year, seq int, err error) {
	if len(id) != 7 || id[:2] != "CP" {
		return 0, 0, fmt.Errorf("invalid forecast id: %q", id)
	}
	yy, err := strconv.Atoi(id[2:4])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid forecast id year: %q", id)
	}
	year = 2000 + yy
	tail := id[4:7]
	if tail[0] >= '0' && tail[0] <= '9' {
		n, err := strconv.Atoi(tail)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
		}
		return year, n, nil
	}
	if tail[0] < 'A' || tail[0] > 'Z' {
		return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
	}
	rest, err := strconv.Atoi(tail[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid forecast id sequence: %q", id)
	}
	return year, 1000 + int(tail[0]-'A')*100 + rest, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/forecastid/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/forecastid/
git commit -m "feat(forecastid): CPYYnnn format/parse with letter overflow"
```

---

### Task 4: Core entities

**Files:**
- Create: `internal/domain/model/partner.go`, `section.go`, `taxonomy.go`, `window.go`
- Test: `internal/domain/model/partner_test.go`, `window_test.go`

**Interfaces:**
- Produces (constructors return `(T, error)` where validation exists; accessors are methods):
  - `Partner`: `NewPartner(id int, name, surname, vatCode, email, mobile string, pt PartnerType, riaNumber int, addedOn time.Time, boardMember bool) (Partner, error)`; accessors `ID,Name,Surname,VatCode,Email,Mobile,PartnerType,RiaNumber,AddedOn,BoardMember`; `WithBoardMember(bool) Partner`.
  - `Section`: `NewSection(code, label string, active bool, displayOrder int) (Section, error)`; accessors `Code,Label,Active,DisplayOrder`.
  - `PartnerSection`: `NewPartnerSection(partnerID int, sectionCode string) (PartnerSection, error)`; accessors `PartnerID,SectionCode`.
  - `ExpenseType`: `NewExpenseType(year int, code, label string, cat ExpenseCategory) (ExpenseType, error)`; accessors `Year,Code,Label,Category`.
  - `ExpenseSubtype`: `NewExpenseSubtype(year int, code, label, typeCode string) (ExpenseSubtype, error)`; accessors `Year,Code,Label,TypeCode`.
  - `SubmissionWindow`: `NewSubmissionWindow(year int, state WindowState, openedAt, closedAt *time.Time, deadline time.Time, current, investment Money) (SubmissionWindow, error)`; accessors `Year,State,OpenedAt,ClosedAt,Deadline,CurrentExpenseLimit,InvestmentExpenseLimit`; `WithState(WindowState)`, `WithOpenedAt(time.Time)`, `WithClosedAt(time.Time)`. (`openedAt`/`closedAt` are `*time.Time`, nil when unset.)

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/model/partner_test.go`:
```go
package model

import (
	"testing"
	"time"
)

func TestNewPartner_Valid(t *testing.T) {
	p, err := NewPartner(1, "Pau", "Bosch", "X1", "p@e.cat", "600", Productor, 13937,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != 1 || p.Name() != "Pau" || p.PartnerType() != Productor {
		t.Errorf("accessors wrong: %+v", p)
	}
	if p.WithBoardMember(true).BoardMember() != true {
		t.Error("WithBoardMember failed")
	}
	if p.BoardMember() != false {
		t.Error("WithBoardMember mutated the original")
	}
}

func TestNewPartner_RejectsNegativeID(t *testing.T) {
	if _, err := NewPartner(-1, "x", "y", "v", "e", "m", Productor, 0, time.Now(), false); err == nil {
		t.Fatal("expected error for negative id")
	}
}
```

Create `internal/domain/model/window_test.go`:
```go
package model

import (
	"testing"
	"time"
)

func TestNewSubmissionWindow_AndWith(t *testing.T) {
	deadline := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	w, err := NewSubmissionWindow(2026, WindowDraft, nil, nil, deadline, MoneyOf(30000), MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	if w.Year() != 2026 || w.State() != WindowDraft {
		t.Errorf("accessors wrong: %+v", w)
	}
	opened := w.WithState(WindowOpen)
	if opened.State() != WindowOpen || w.State() != WindowDraft {
		t.Error("WithState should not mutate original")
	}
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if got := w.WithOpenedAt(now).OpenedAt(); got == nil || !got.Equal(now) {
		t.Errorf("WithOpenedAt = %v, want %v", got, now)
	}
}

func TestNewSubmissionWindow_RejectsBadYear(t *testing.T) {
	if _, err := NewSubmissionWindow(1800, WindowDraft, nil, nil, time.Now(), ZeroMoney(), ZeroMoney()); err == nil {
		t.Fatal("expected error for year < 1900")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/model/ -run 'TestNewPartner|TestNewSubmissionWindow' -v`
Expected: FAIL — undefined constructors.

- [ ] **Step 3: Write partner.go**

Create `internal/domain/model/partner.go`:
```go
package model

import (
	"fmt"
	"time"
)

type Partner struct {
	id           int
	name         string
	surname      string
	vatCode      string
	email        string
	mobile       string
	partnerType  PartnerType
	riaNumber    int
	addedOn      time.Time
	boardMember  bool
}

func NewPartner(id int, name, surname, vatCode, email, mobile string, pt PartnerType,
	riaNumber int, addedOn time.Time, boardMember bool) (Partner, error) {
	if id < 0 {
		return Partner{}, fmt.Errorf("partner id must be >= 0, got %d", id)
	}
	if riaNumber < 0 {
		return Partner{}, fmt.Errorf("riaNumber must be >= 0, got %d", riaNumber)
	}
	return Partner{id, name, surname, vatCode, email, mobile, pt, riaNumber, addedOn, boardMember}, nil
}

func (p Partner) ID() int                  { return p.id }
func (p Partner) Name() string             { return p.name }
func (p Partner) Surname() string          { return p.surname }
func (p Partner) VatCode() string          { return p.vatCode }
func (p Partner) Email() string            { return p.email }
func (p Partner) Mobile() string           { return p.mobile }
func (p Partner) PartnerType() PartnerType { return p.partnerType }
func (p Partner) RiaNumber() int           { return p.riaNumber }
func (p Partner) AddedOn() time.Time       { return p.addedOn }
func (p Partner) BoardMember() bool        { return p.boardMember }

func (p Partner) WithBoardMember(b bool) Partner {
	p.boardMember = b
	return p
}
```

- [ ] **Step 4: Write section.go**

Create `internal/domain/model/section.go`:
```go
package model

import "fmt"

type Section struct {
	code         string
	label        string
	active       bool
	displayOrder int
}

func NewSection(code, label string, active bool, displayOrder int) (Section, error) {
	if code == "" {
		return Section{}, fmt.Errorf("section code must not be empty")
	}
	if label == "" {
		return Section{}, fmt.Errorf("section label must not be empty")
	}
	return Section{code, label, active, displayOrder}, nil
}

func (s Section) Code() string      { return s.code }
func (s Section) Label() string     { return s.label }
func (s Section) Active() bool       { return s.active }
func (s Section) DisplayOrder() int { return s.displayOrder }

type PartnerSection struct {
	partnerID   int
	sectionCode string
}

func NewPartnerSection(partnerID int, sectionCode string) (PartnerSection, error) {
	if partnerID < 0 {
		return PartnerSection{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	if sectionCode == "" {
		return PartnerSection{}, fmt.Errorf("sectionCode must not be empty")
	}
	return PartnerSection{partnerID, sectionCode}, nil
}

func (m PartnerSection) PartnerID() int      { return m.partnerID }
func (m PartnerSection) SectionCode() string { return m.sectionCode }
```

- [ ] **Step 5: Write taxonomy.go**

Create `internal/domain/model/taxonomy.go`:
```go
package model

import "fmt"

type ExpenseType struct {
	year     int
	code     string
	label    string
	category ExpenseCategory
}

func NewExpenseType(year int, code, label string, cat ExpenseCategory) (ExpenseType, error) {
	if code == "" {
		return ExpenseType{}, fmt.Errorf("expense type code must not be empty")
	}
	if _, err := ParseExpenseCategory(string(cat)); err != nil {
		return ExpenseType{}, err
	}
	return ExpenseType{year, code, label, cat}, nil
}

func (t ExpenseType) Year() int                { return t.year }
func (t ExpenseType) Code() string             { return t.code }
func (t ExpenseType) Label() string            { return t.label }
func (t ExpenseType) Category() ExpenseCategory { return t.category }

type ExpenseSubtype struct {
	year     int
	code     string
	label    string
	typeCode string
}

func NewExpenseSubtype(year int, code, label, typeCode string) (ExpenseSubtype, error) {
	if code == "" {
		return ExpenseSubtype{}, fmt.Errorf("expense subtype code must not be empty")
	}
	if typeCode == "" {
		return ExpenseSubtype{}, fmt.Errorf("expense subtype typeCode must not be empty")
	}
	return ExpenseSubtype{year, code, label, typeCode}, nil
}

func (s ExpenseSubtype) Year() int        { return s.year }
func (s ExpenseSubtype) Code() string     { return s.code }
func (s ExpenseSubtype) Label() string    { return s.label }
func (s ExpenseSubtype) TypeCode() string { return s.typeCode }
```

- [ ] **Step 6: Write window.go**

Create `internal/domain/model/window.go`:
```go
package model

import (
	"fmt"
	"time"
)

type SubmissionWindow struct {
	year                   int
	state                  WindowState
	openedAt               *time.Time
	closedAt               *time.Time
	deadline               time.Time
	currentExpenseLimit    Money
	investmentExpenseLimit Money
}

func NewSubmissionWindow(year int, state WindowState, openedAt, closedAt *time.Time,
	deadline time.Time, current, investment Money) (SubmissionWindow, error) {
	if year < 1900 {
		return SubmissionWindow{}, fmt.Errorf("year out of range: %d", year)
	}
	if _, err := ParseWindowState(string(state)); err != nil {
		return SubmissionWindow{}, err
	}
	return SubmissionWindow{year, state, openedAt, closedAt, deadline, current, investment}, nil
}

func (w SubmissionWindow) Year() int                       { return w.year }
func (w SubmissionWindow) State() WindowState              { return w.state }
func (w SubmissionWindow) OpenedAt() *time.Time            { return w.openedAt }
func (w SubmissionWindow) ClosedAt() *time.Time            { return w.closedAt }
func (w SubmissionWindow) Deadline() time.Time             { return w.deadline }
func (w SubmissionWindow) CurrentExpenseLimit() Money      { return w.currentExpenseLimit }
func (w SubmissionWindow) InvestmentExpenseLimit() Money   { return w.investmentExpenseLimit }

func (w SubmissionWindow) WithState(s WindowState) SubmissionWindow { w.state = s; return w }
func (w SubmissionWindow) WithOpenedAt(t time.Time) SubmissionWindow { w.openedAt = &t; return w }
func (w SubmissionWindow) WithClosedAt(t time.Time) SubmissionWindow { w.closedAt = &t; return w }
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/domain/model/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/model/partner.go internal/domain/model/section.go internal/domain/model/taxonomy.go internal/domain/model/window.go internal/domain/model/partner_test.go internal/domain/model/window_test.go
git commit -m "feat(model): add Partner, Section, taxonomy, and SubmissionWindow entities"
```

---

### Task 5: Transactional entities

**Files:**
- Create: `internal/domain/model/forecast.go`, `report.go`, `audit.go`, `board.go`
- Test: `internal/domain/model/forecast_test.go`, `board_test.go`

**Interfaces:**
- Produces:
  - `ExpenseForecast`: `NewExpenseForecast(id string, partnerID int, concept, description string, gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int, subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error)`; accessors `ID,PartnerID,Concept,Description,GrossAmount,ApprovedAmount,ApprovedOn,PlannedDate,Year,SubtypeCode,Scope,AddedOn,Enabled`; `WithApprovedAmount(Money)`, `WithApprovedOn(time.Time)`, `WithEnabled(bool)`. Invariant: `year == plannedDate.Year()`.
  - `Report`: `NewReport(id, year int, generatedAt time.Time, snapshotJSON string, pdf []byte, supersededAt *time.Time) (Report, error)`; accessors `ID,Year,GeneratedAt,SnapshotJSON,Pdf,SupersededAt`; `WithSupersededAt(time.Time)`.
  - `AuditEvent`: `NewAuditEvent(id int, actorID *int, actorEmail string, kind AuditKind, entityType, entityID string, timestamp time.Time, payload *string) (AuditEvent, error)`; accessors `ID,ActorID,ActorEmail,Kind,EntityType,EntityID,Timestamp,Payload`.
  - `BoardAuthorization`: `NewBoardAuthorization(partnerID int, scopeKind ScopeKind, sectionCode string) (BoardAuthorization, error)` — only `COMMON`/`SECTION`; sectionCode iff SECTION; accessors `PartnerID,ScopeKind,SectionCode`.

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/model/forecast_test.go`:
```go
package model

import (
	"testing"
	"time"
)

func TestNewExpenseForecast_Valid(t *testing.T) {
	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	added := time.Date(2026, 2, 21, 19, 0, 0, 0, time.UTC)
	f, err := NewExpenseForecast("CP26023", 7, "Projecte", "desc", MoneyOf(2880), MoneyOf(2880),
		nil, planned, 2026, "a2", NewCommonScope(), added, true)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID() != "CP26023" || f.Year() != 2026 || f.Scope().Kind() != ScopeCommon {
		t.Errorf("accessors wrong: %+v", f)
	}
	f2 := f.WithApprovedAmount(MoneyOf(2000))
	if f2.ApprovedAmount().String() != "2000.00" || f.ApprovedAmount().String() != "2880.00" {
		t.Error("WithApprovedAmount should not mutate original")
	}
}

func TestNewExpenseForecast_YearMustMatchPlannedDate(t *testing.T) {
	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := NewExpenseForecast("CP25001", 1, "c", "d", ZeroMoney(), ZeroMoney(),
		nil, planned, 2025, "a1", NewPartnerScope(), time.Now(), true)
	if err == nil {
		t.Fatal("expected error: year 2025 != plannedDate.Year() 2026")
	}
}
```

Create `internal/domain/model/board_test.go`:
```go
package model

import "testing"

func TestNewBoardAuthorization(t *testing.T) {
	a, err := NewBoardAuthorization(7, ScopeSection, "oliva")
	if err != nil || a.ScopeKind() != ScopeSection || a.SectionCode() != "oliva" {
		t.Fatalf("got (%+v,%v)", a, err)
	}
	if _, err := NewBoardAuthorization(7, ScopeCommon, ""); err != nil {
		t.Errorf("COMMON without section should be valid: %v", err)
	}
	if _, err := NewBoardAuthorization(7, ScopePartner, ""); err == nil {
		t.Error("PARTNER scope must be rejected for board authorization")
	}
	if _, err := NewBoardAuthorization(7, ScopeSection, ""); err == nil {
		t.Error("SECTION without code must be rejected")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/model/ -run 'TestNewExpenseForecast|TestNewBoardAuthorization' -v`
Expected: FAIL — undefined constructors.

- [ ] **Step 3: Write forecast.go**

Create `internal/domain/model/forecast.go`:
```go
package model

import (
	"fmt"
	"time"
)

type ExpenseForecast struct {
	id             string
	partnerID      int
	concept        string
	description    string
	grossAmount    Money
	approvedAmount Money
	approvedOn     *time.Time
	plannedDate    time.Time
	year           int
	subtypeCode    string
	scope          ExpenseScope
	addedOn        time.Time
	enabled        bool
}

func NewExpenseForecast(id string, partnerID int, concept, description string,
	gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int,
	subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error) {
	if id == "" {
		return ExpenseForecast{}, fmt.Errorf("forecast id must not be empty")
	}
	if partnerID < 0 {
		return ExpenseForecast{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	if subtypeCode == "" {
		return ExpenseForecast{}, fmt.Errorf("subtypeCode must not be empty")
	}
	if year != plannedDate.Year() {
		return ExpenseForecast{}, fmt.Errorf("year %d must equal plannedDate year %d", year, plannedDate.Year())
	}
	return ExpenseForecast{id, partnerID, concept, description, gross, approved,
		approvedOn, plannedDate, year, subtypeCode, scope, addedOn, enabled}, nil
}

func (f ExpenseForecast) ID() string           { return f.id }
func (f ExpenseForecast) PartnerID() int        { return f.partnerID }
func (f ExpenseForecast) Concept() string       { return f.concept }
func (f ExpenseForecast) Description() string    { return f.description }
func (f ExpenseForecast) GrossAmount() Money     { return f.grossAmount }
func (f ExpenseForecast) ApprovedAmount() Money  { return f.approvedAmount }
func (f ExpenseForecast) ApprovedOn() *time.Time { return f.approvedOn }
func (f ExpenseForecast) PlannedDate() time.Time { return f.plannedDate }
func (f ExpenseForecast) Year() int              { return f.year }
func (f ExpenseForecast) SubtypeCode() string    { return f.subtypeCode }
func (f ExpenseForecast) Scope() ExpenseScope    { return f.scope }
func (f ExpenseForecast) AddedOn() time.Time     { return f.addedOn }
func (f ExpenseForecast) Enabled() bool          { return f.enabled }

func (f ExpenseForecast) WithApprovedAmount(m Money) ExpenseForecast { f.approvedAmount = m; return f }
func (f ExpenseForecast) WithApprovedOn(t time.Time) ExpenseForecast { f.approvedOn = &t; return f }
func (f ExpenseForecast) WithEnabled(b bool) ExpenseForecast         { f.enabled = b; return f }
```

- [ ] **Step 4: Write report.go**

Create `internal/domain/model/report.go`:
```go
package model

import (
	"fmt"
	"time"
)

type Report struct {
	id           int
	year         int
	generatedAt  time.Time
	snapshotJSON string
	pdf          []byte
	supersededAt *time.Time
}

func NewReport(id, year int, generatedAt time.Time, snapshotJSON string, pdf []byte,
	supersededAt *time.Time) (Report, error) {
	if snapshotJSON == "" {
		return Report{}, fmt.Errorf("report snapshotJSON must not be empty")
	}
	return Report{id, year, generatedAt, snapshotJSON, pdf, supersededAt}, nil
}

func (r Report) ID() int                { return r.id }
func (r Report) Year() int              { return r.year }
func (r Report) GeneratedAt() time.Time { return r.generatedAt }
func (r Report) SnapshotJSON() string   { return r.snapshotJSON }
func (r Report) Pdf() []byte            { return r.pdf }
func (r Report) SupersededAt() *time.Time { return r.supersededAt }

func (r Report) WithSupersededAt(t time.Time) Report { r.supersededAt = &t; return r }
```

- [ ] **Step 5: Write audit.go**

Create `internal/domain/model/audit.go`:
```go
package model

import (
	"fmt"
	"time"
)

type AuditEvent struct {
	id         int
	actorID    *int
	actorEmail string
	kind       AuditKind
	entityType string
	entityID   string
	timestamp  time.Time
	payload    *string
}

func NewAuditEvent(id int, actorID *int, actorEmail string, kind AuditKind,
	entityType, entityID string, timestamp time.Time, payload *string) (AuditEvent, error) {
	if actorEmail == "" {
		return AuditEvent{}, fmt.Errorf("actorEmail must not be empty")
	}
	if _, err := ParseAuditKind(string(kind)); err != nil {
		return AuditEvent{}, err
	}
	return AuditEvent{id, actorID, actorEmail, kind, entityType, entityID, timestamp, payload}, nil
}

func (e AuditEvent) ID() int            { return e.id }
func (e AuditEvent) ActorID() *int      { return e.actorID }
func (e AuditEvent) ActorEmail() string { return e.actorEmail }
func (e AuditEvent) Kind() AuditKind    { return e.kind }
func (e AuditEvent) EntityType() string { return e.entityType }
func (e AuditEvent) EntityID() string   { return e.entityID }
func (e AuditEvent) Timestamp() time.Time { return e.timestamp }
func (e AuditEvent) Payload() *string    { return e.payload }
```

- [ ] **Step 6: Write board.go**

Create `internal/domain/model/board.go`:
```go
package model

import "fmt"

type BoardAuthorization struct {
	partnerID   int
	scopeKind   ScopeKind
	sectionCode string
}

// NewBoardAuthorization builds an authorization for a non-Soci scope a board
// member may edit on the web. Only COMMON and SECTION are valid; sectionCode is
// set iff scopeKind == SECTION.
func NewBoardAuthorization(partnerID int, scopeKind ScopeKind, sectionCode string) (BoardAuthorization, error) {
	if partnerID < 0 {
		return BoardAuthorization{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	switch scopeKind {
	case ScopeCommon:
		if sectionCode != "" {
			return BoardAuthorization{}, fmt.Errorf("COMMON authorization must not carry a section code")
		}
	case ScopeSection:
		if sectionCode == "" {
			return BoardAuthorization{}, fmt.Errorf("SECTION authorization requires a section code")
		}
	default:
		return BoardAuthorization{}, fmt.Errorf("board authorization scope must be COMMON or SECTION, got %q", scopeKind)
	}
	return BoardAuthorization{partnerID, scopeKind, sectionCode}, nil
}

func (a BoardAuthorization) PartnerID() int       { return a.partnerID }
func (a BoardAuthorization) ScopeKind() ScopeKind { return a.scopeKind }
func (a BoardAuthorization) SectionCode() string  { return a.sectionCode }
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/domain/model/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/model/forecast.go internal/domain/model/report.go internal/domain/model/audit.go internal/domain/model/board.go internal/domain/model/forecast_test.go internal/domain/model/board_test.go
git commit -m "feat(model): add ExpenseForecast, Report, AuditEvent, BoardAuthorization"
```

---

### Task 6: Domain ports + system Clock

**Files:**
- Create: `internal/domain/ports/ports.go`
- Create: `internal/adapters/system/clock.go`
- Test: `internal/adapters/system/clock_test.go`

**Interfaces:**
- Produces in `internal/domain/ports` (interfaces; `model` is the domain model package):
  - `Clock interface { Now() time.Time }`
  - `PartnerRepository`, `SectionRepository`, `TaxonomyRepository`, `WindowRepository`, `ForecastRepository`, `ReportRepository`, `AuditLog`, `BoardAuthorizationRepository` — method sets given below (implemented in Tasks 8–14).
- Produces `system.SystemClock` implementing `ports.Clock` (UTC).

The port method sets (use these exact signatures in Tasks 8–14):
```go
PartnerRepository interface {
	Save(ctx context.Context, p model.Partner) error
	FindByID(ctx context.Context, id int) (model.Partner, bool, error)
	FindByEmail(ctx context.Context, email string) (model.Partner, bool, error)
	List(ctx context.Context) ([]model.Partner, error)
}
SectionRepository interface {
	Save(ctx context.Context, s model.Section) error
	List(ctx context.Context) ([]model.Section, error)
	AddMembership(ctx context.Context, m model.PartnerSection) error
	ListMembershipsByPartner(ctx context.Context, partnerID int) ([]model.PartnerSection, error)
}
TaxonomyRepository interface {
	SaveType(ctx context.Context, t model.ExpenseType) error
	SaveSubtype(ctx context.Context, s model.ExpenseSubtype) error
	ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error)
	ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error)
}
WindowRepository interface {
	Save(ctx context.Context, w model.SubmissionWindow) error
	FindByYear(ctx context.Context, year int) (model.SubmissionWindow, bool, error)
	List(ctx context.Context) ([]model.SubmissionWindow, error)
}
ForecastRepository interface {
	// Create inserts a new forecast, allocating the next CPYYnnn id for its year,
	// and returns the stored forecast (with its id set).
	Create(ctx context.Context, f model.ExpenseForecast) (model.ExpenseForecast, error)
	Save(ctx context.Context, f model.ExpenseForecast) error // update existing by id
	FindByID(ctx context.Context, id string) (model.ExpenseForecast, bool, error)
	ListByYear(ctx context.Context, year int) ([]model.ExpenseForecast, error)
}
ReportRepository interface {
	Insert(ctx context.Context, r model.Report) (int, error) // returns new id
	FindLatestByYear(ctx context.Context, year int) (model.Report, bool, error)
	MarkSuperseded(ctx context.Context, id int, at time.Time) error
}
AuditLog interface {
	Append(ctx context.Context, e model.AuditEvent) error
	List(ctx context.Context) ([]model.AuditEvent, error)
}
BoardAuthorizationRepository interface {
	Save(ctx context.Context, a model.BoardAuthorization) error
	ListByPartner(ctx context.Context, partnerID int) ([]model.BoardAuthorization, error)
}
```

- [ ] **Step 1: Write the failing test (Clock)**

Create `internal/adapters/system/clock_test.go`:
```go
package system

import (
	"testing"
	"time"
)

func TestSystemClock_NowIsUTCAndRecent(t *testing.T) {
	c := SystemClock{}
	now := c.Now()
	if now.Location() != time.UTC {
		t.Errorf("Now() location = %v, want UTC", now.Location())
	}
	if time.Since(now) > time.Minute {
		t.Errorf("Now() = %v is not recent", now)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/system/ -v`
Expected: FAIL — undefined `SystemClock`.

- [ ] **Step 3: Write ports.go**

Create `internal/domain/ports/ports.go` with the `package ports`, imports `context`, `time`, and `github.com/pjover/espigol/internal/domain/model`, and the `Clock` interface plus all repository interfaces exactly as listed in the Interfaces block above.

- [ ] **Step 4: Write the Clock adapter**

Create `internal/adapters/system/clock.go`:
```go
// Package system holds adapters for system facilities (clock, etc.).
package system

import "time"

// SystemClock implements ports.Clock using the wall clock in UTC.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }
```

- [ ] **Step 5: Run test + build**

Run: `go test ./internal/adapters/system/ -v && go build ./...`
Expected: PASS, build clean (ports compiles).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/ports/ports.go internal/adapters/system/
git commit -m "feat(ports): define repository ports and system Clock"
```

---

### Task 7: Persistence foundation (goose schema, DB open, sqlc setup)

**Files:**
- Create: `db/migrations/00001_init.sql`
- Create: `db/migrations/embed.go`
- Create: `internal/adapters/persistence/db/db.go`
- Create: `sqlc.yaml`
- Create: `db/queries/.keep` (empty placeholder so sqlc has a queries dir)
- Test: `internal/adapters/persistence/db/db_test.go`
- Modify: `Makefile` (add `sqlc-generate`, `migrate-status` targets)

**Interfaces:**
- Consumes: nothing from earlier tasks except module layout.
- Produces:
  - `migrations.FS` (embedded `embed.FS`) and the migrations dir.
  - `db.Open(path string) (*sql.DB, error)` — opens modernc sqlite with WAL + busy_timeout + foreign_keys pragmas and runs goose migrations to latest.
  - the sqlc config so Tasks 8–14 can run `make sqlc-generate`.

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get modernc.org/sqlite@latest
go get github.com/pressly/goose/v3@latest
```
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the goose migration**

Create `db/migrations/00001_init.sql`:
```sql
-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE partner (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    surname      TEXT NOT NULL,
    vat_code     TEXT NOT NULL,
    email        TEXT NOT NULL UNIQUE,
    mobile       TEXT NOT NULL,
    partner_type TEXT NOT NULL,
    ria_number   INTEGER NOT NULL,
    added_on     TEXT NOT NULL,
    board_member INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE section (
    code          TEXT PRIMARY KEY,
    label         TEXT NOT NULL,
    active        INTEGER NOT NULL DEFAULT 1,
    display_order INTEGER NOT NULL
);

CREATE TABLE partner_section (
    partner_id   INTEGER NOT NULL,
    section_code TEXT NOT NULL,
    PRIMARY KEY (partner_id, section_code),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE TABLE submission_window (
    year                     INTEGER PRIMARY KEY,
    state                    TEXT NOT NULL CHECK (state IN ('DRAFT','OPEN','CLOSED')),
    opened_at                TEXT,
    closed_at                TEXT,
    deadline                 TEXT NOT NULL,
    current_expense_limit    TEXT NOT NULL,
    investment_expense_limit TEXT NOT NULL
);

CREATE UNIQUE INDEX one_open_window
    ON submission_window(state) WHERE state = 'OPEN';

CREATE TABLE expense_type (
    year     INTEGER NOT NULL,
    code     TEXT NOT NULL,
    label    TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('CURRENT','INVESTMENT')),
    PRIMARY KEY (year, code),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE TABLE expense_subtype (
    year      INTEGER NOT NULL,
    code      TEXT NOT NULL,
    label     TEXT NOT NULL,
    type_code TEXT NOT NULL,
    PRIMARY KEY (year, code),
    FOREIGN KEY (year, type_code) REFERENCES expense_type(year, code)
);

CREATE TABLE expense_forecast (
    id              TEXT PRIMARY KEY,
    partner_id      INTEGER NOT NULL,
    concept         TEXT NOT NULL,
    description     TEXT NOT NULL,
    gross_amount    TEXT NOT NULL,
    approved_amount TEXT NOT NULL,
    approved_on     TEXT,
    planned_date    TEXT NOT NULL,
    year            INTEGER NOT NULL,
    subtype_code    TEXT NOT NULL,
    scope_kind      TEXT NOT NULL CHECK (scope_kind IN ('COMMON','SECTION','PARTNER')),
    section_code    TEXT,
    added_on        TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    CHECK ((scope_kind = 'SECTION') = (section_code IS NOT NULL)),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (year) REFERENCES submission_window(year),
    FOREIGN KEY (year, subtype_code) REFERENCES expense_subtype(year, code),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE INDEX idx_forecast_year_enabled ON expense_forecast(year, enabled);
CREATE INDEX idx_forecast_partner ON expense_forecast(partner_id);

CREATE TABLE report (
    id            INTEGER PRIMARY KEY,
    year          INTEGER NOT NULL,
    generated_at  TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    pdf           BLOB NOT NULL,
    superseded_at TEXT,
    UNIQUE (year, generated_at),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE INDEX idx_report_latest_per_year
    ON report(year, generated_at) WHERE superseded_at IS NULL;

CREATE TABLE audit_event (
    id          INTEGER PRIMARY KEY,
    actor_id    INTEGER,
    actor_email TEXT NOT NULL,
    kind        TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    timestamp   TEXT NOT NULL,
    payload     TEXT,
    FOREIGN KEY (actor_id) REFERENCES partner(id)
);

CREATE INDEX idx_audit_timestamp ON audit_event(timestamp DESC);
CREATE INDEX idx_audit_entity ON audit_event(entity_type, entity_id);

CREATE TABLE board_authorization (
    partner_id   INTEGER NOT NULL,
    scope_kind   TEXT NOT NULL CHECK (scope_kind IN ('COMMON','SECTION')),
    section_code TEXT,
    CHECK ((scope_kind = 'SECTION') = (section_code IS NOT NULL)),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE UNIQUE INDEX uq_board_authorization
    ON board_authorization(partner_id, scope_kind, COALESCE(section_code, ''));

-- +goose Down
DROP TABLE board_authorization;
DROP TABLE audit_event;
DROP TABLE report;
DROP TABLE expense_forecast;
DROP TABLE expense_subtype;
DROP TABLE expense_type;
DROP TABLE submission_window;
DROP TABLE partner_section;
DROP TABLE section;
DROP TABLE partner;
```

- [ ] **Step 3: Embed the migrations**

Create `db/migrations/embed.go`:
```go
// Package migrations embeds the goose SQL migrations.
package migrations

import "embed"

// FS holds the embedded migration files.
//
//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 4: Write the failing DB test**

Create `internal/adapters/persistence/db/db_test.go`:
```go
package db

import (
	"path/filepath"
	"testing"
)

func TestOpen_MigratesAndEnablesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "espigol.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// foreign_keys pragma must be ON for every connection.
	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// journal mode must be WAL.
	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}

	// migrations created the core tables.
	var n int
	if err := conn.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('partner','section','expense_forecast','board_authorization')",
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("expected 4 core tables, found %d", n)
	}
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/db/ -v`
Expected: FAIL — undefined `Open`.

- [ ] **Step 6: Write the DB open + migrate code**

Create `internal/adapters/persistence/db/db.go`:
```go
// Package db opens the espigol SQLite database (pure-Go modernc driver),
// configures per-connection pragmas, and runs goose migrations to latest.
package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	migrations "github.com/pjover/espigol/db/migrations"
	_ "modernc.org/sqlite"
)

// Open opens the database at path with WAL, busy_timeout, and foreign_keys
// pragmas applied to every pooled connection, then runs migrations to latest.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		path,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}
```

Note: the embedded FS roots at the package dir, so the migrations dir passed to `goose.Up` is `"."`. If goose cannot find versions, adjust `//go:embed` to `migrations/*.sql` and use a sub-FS — but with `embed.go` living beside the `.sql` files, `"."` is correct.

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/db/ -v`
Expected: PASS.

- [ ] **Step 8: Write sqlc config**

Create `sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: "sqlite"
    schema: "db/migrations"
    queries: "db/queries"
    gen:
      go:
        package: "sqlc"
        out: "internal/adapters/persistence/sqlc"
        emit_json_tags: false
        emit_interface: false
        emit_empty_slices: true
```

Create an empty `db/queries/.keep` file so the directory exists.

- [ ] **Step 9: Add Makefile targets**

Add to `Makefile` (under `.PHONY` and as new targets; recipes are TAB-indented):
```makefile
sqlc-generate:
	go tool sqlc generate

migrate-status:
	@echo "migrations are applied automatically on Open; see db/migrations/"
```
Add `sqlc-generate migrate-status` to the `.PHONY` line.

Install sqlc as a module tool so the build is reproducible:
```bash
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```
Expected: adds a `tool` directive to `go.mod`. `make sqlc-generate` runs with no queries yet → produces models in `internal/adapters/persistence/sqlc` (or a no-op message); commit whatever it emits.

- [ ] **Step 10: Verify build + tests + tidy**

Run: `go mod tidy && go build ./... && go test ./...`
Expected: all green.

- [ ] **Step 11: Commit**

```bash
git add db/ sqlc.yaml internal/adapters/persistence/ Makefile go.mod go.sum
git commit -m "feat(persistence): goose schema, DB open with pragmas, sqlc setup"
```

---

### Task 8: Partner repository

**Files:**
- Create: `db/queries/partner.sql`
- Create: `internal/adapters/persistence/mapper/timeconv.go` (shared time/date helpers)
- Create: `internal/adapters/persistence/mapper/partner.go`
- Create: `internal/adapters/persistence/partner_repository.go`
- Test: `internal/adapters/persistence/mapper/timeconv_test.go`
- Test: `internal/adapters/persistence/partner_repository_test.go`

**Interfaces:**
- Consumes: `db.Open`, `model.Partner`, `ports.PartnerRepository`.
- Produces:
  - `mapper` shared helpers: `FormatTimestamp(time.Time) string` (RFC3339 UTC), `ParseTimestamp(string) (time.Time, error)`, `FormatDate(time.Time) string` (`2006-01-02`), `ParseDate(string) (time.Time, error)`, and nullable variants `FormatNullableTimestamp(*time.Time) sql.NullString`, `ParseNullableTimestamp(sql.NullString) (*time.Time, error)`.
  - `persistence.NewPartnerRepository(q *sqlc.Queries) *PartnerRepository` implementing `ports.PartnerRepository`.

- [ ] **Step 1: Write partner queries and generate**

Create `db/queries/partner.sql`:
```sql
-- name: UpsertPartner :exec
INSERT INTO partner (id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name, surname=excluded.surname, vat_code=excluded.vat_code,
  email=excluded.email, mobile=excluded.mobile, partner_type=excluded.partner_type,
  ria_number=excluded.ria_number, added_on=excluded.added_on, board_member=excluded.board_member;

-- name: GetPartner :one
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member
FROM partner WHERE id = ?;

-- name: GetPartnerByEmail :one
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member
FROM partner WHERE email = ?;

-- name: ListPartners :many
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member
FROM partner ORDER BY id;
```

Run: `make sqlc-generate`
Expected: regenerates `internal/adapters/persistence/sqlc` with `Partner` row struct and the four query methods.

- [ ] **Step 2: Write the failing time-helpers test**

Create `internal/adapters/persistence/mapper/timeconv_test.go`:
```go
package mapper

import (
	"testing"
	"time"
)

func TestTimestampRoundTrip(t *testing.T) {
	in := time.Date(2026, 3, 1, 18, 36, 37, 0, time.UTC)
	out, err := ParseTimestamp(FormatTimestamp(in))
	if err != nil || !out.Equal(in) {
		t.Fatalf("round trip: got (%v,%v), want %v", out, err, in)
	}
}

func TestDateRoundTrip(t *testing.T) {
	in := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	s := FormatDate(in)
	if s != "2026-03-01" {
		t.Errorf("FormatDate = %q, want 2026-03-01", s)
	}
	out, err := ParseDate(s)
	if err != nil || !out.Equal(in) {
		t.Fatalf("date round trip: got (%v,%v), want %v", out, err, in)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/mapper/ -v`
Expected: FAIL — undefined `FormatTimestamp`.

- [ ] **Step 4: Write the time helpers**

Create `internal/adapters/persistence/mapper/timeconv.go`:
```go
// Package mapper translates between sqlc row structs and domain types.
package mapper

import (
	"database/sql"
	"time"
)

const dateLayout = "2006-01-02"

func FormatTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func FormatDate(t time.Time) string { return t.UTC().Format(dateLayout) }

func ParseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, s)
}

func FormatNullableTimestamp(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: FormatTimestamp(*t), Valid: true}
}

func ParseNullableTimestamp(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := ParseTimestamp(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
```

- [ ] **Step 5: Run helper test to verify it passes**

Run: `go test ./internal/adapters/persistence/mapper/ -v`
Expected: PASS.

- [ ] **Step 6: Write the partner mapper**

Create `internal/adapters/persistence/mapper/partner.go`:
```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func PartnerToRow(p model.Partner) sqlc.UpsertPartnerParams {
	board := int64(0)
	if p.BoardMember() {
		board = 1
	}
	return sqlc.UpsertPartnerParams{
		ID:          int64(p.ID()),
		Name:        p.Name(),
		Surname:     p.Surname(),
		VatCode:     p.VatCode(),
		Email:       p.Email(),
		Mobile:      p.Mobile(),
		PartnerType: string(p.PartnerType()),
		RiaNumber:   int64(p.RiaNumber()),
		AddedOn:     FormatDate(p.AddedOn()),
		BoardMember: board,
	}
}

func PartnerFromRow(r sqlc.Partner) (model.Partner, error) {
	pt, err := model.ParsePartnerType(r.PartnerType)
	if err != nil {
		return model.Partner{}, err
	}
	addedOn, err := ParseDate(r.AddedOn)
	if err != nil {
		return model.Partner{}, err
	}
	return model.NewPartner(int(r.ID), r.Name, r.Surname, r.VatCode, r.Email, r.Mobile,
		pt, int(r.RiaNumber), addedOn, r.BoardMember == 1)
}
```

(If sqlc names a generated field differently — e.g. it pluralizes or types `int64` vs `sql.NullInt64` — adjust these references to the actual generated identifiers. Run `make sqlc-generate` and read `internal/adapters/persistence/sqlc` for exact names.)

- [ ] **Step 7: Write the failing repository test**

Create `internal/adapters/persistence/partner_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func openTestDB(t *testing.T) *sqlc.Queries {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return sqlc.New(conn)
}

func TestPartnerRepository_RoundTrip(t *testing.T) {
	repo := persistence.NewPartnerRepository(openTestDB(t))
	ctx := context.Background()

	p, _ := model.NewPartner(1, "Pau", "Bosch Palmer", "X1", "pau@e.cat", "600",
		model.Productor, 13937, time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), true)
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := repo.FindByID(ctx, 1)
	if err != nil || !found {
		t.Fatalf("FindByID: (%v, found=%v)", err, found)
	}
	if got.Name() != "Pau" || got.PartnerType() != model.Productor || !got.BoardMember() {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if !got.AddedOn().Equal(p.AddedOn()) {
		t.Errorf("AddedOn = %v, want %v", got.AddedOn(), p.AddedOn())
	}

	byEmail, found, err := repo.FindByEmail(ctx, "pau@e.cat")
	if err != nil || !found || byEmail.ID() != 1 {
		t.Errorf("FindByEmail: (%+v, %v, %v)", byEmail, found, err)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Errorf("List: len=%d err=%v", len(all), err)
	}
}

func TestPartnerRepository_NotFound(t *testing.T) {
	repo := persistence.NewPartnerRepository(openTestDB(t))
	_, found, err := repo.FindByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for missing partner")
	}
}
```

- [ ] **Step 8: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestPartnerRepository -v`
Expected: FAIL — undefined `NewPartnerRepository`.

- [ ] **Step 9: Write the repository**

Create `internal/adapters/persistence/partner_repository.go`:
```go
// Package persistence holds SQLite repositories implementing the domain ports.
package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type PartnerRepository struct {
	q *sqlc.Queries
}

func NewPartnerRepository(q *sqlc.Queries) *PartnerRepository {
	return &PartnerRepository{q: q}
}

func (r *PartnerRepository) Save(ctx context.Context, p model.Partner) error {
	return r.q.UpsertPartner(ctx, mapper.PartnerToRow(p))
}

func (r *PartnerRepository) FindByID(ctx context.Context, id int) (model.Partner, bool, error) {
	row, err := r.q.GetPartner(ctx, int64(id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Partner{}, false, nil
	}
	if err != nil {
		return model.Partner{}, false, err
	}
	p, err := mapper.PartnerFromRow(row)
	return p, err == nil, err
}

func (r *PartnerRepository) FindByEmail(ctx context.Context, email string) (model.Partner, bool, error) {
	row, err := r.q.GetPartnerByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Partner{}, false, nil
	}
	if err != nil {
		return model.Partner{}, false, err
	}
	p, err := mapper.PartnerFromRow(row)
	return p, err == nil, err
}

func (r *PartnerRepository) List(ctx context.Context) ([]model.Partner, error) {
	rows, err := r.q.ListPartners(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Partner, 0, len(rows))
	for _, row := range rows {
		p, err := mapper.PartnerFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

var _ = func() any { var _ interface {
	Save(context.Context, model.Partner) error
} = (*PartnerRepository)(nil); return nil }()
```

(The trailing compile-time check is optional; you may instead add `var _ ports.PartnerRepository = (*PartnerRepository)(nil)` once you import the ports package. Either way ensure the repo satisfies `ports.PartnerRepository`.)

- [ ] **Step 10: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add db/queries/partner.sql internal/adapters/persistence/ go.mod go.sum
git commit -m "feat(persistence): partner repository + time/date mapper helpers"
```

---

### Task 9: Section repository (sections + membership)

**Files:**
- Create: `db/queries/section.sql`
- Create: `internal/adapters/persistence/mapper/section.go`
- Create: `internal/adapters/persistence/section_repository.go`
- Test: `internal/adapters/persistence/section_repository_test.go`

**Interfaces:**
- Consumes: `db.Open`, `model.Section`, `model.PartnerSection`, `ports.SectionRepository`, `mapper`.
- Produces: `persistence.NewSectionRepository(q *sqlc.Queries) *SectionRepository` implementing `ports.SectionRepository`.

- [ ] **Step 1: Write queries and generate**

Create `db/queries/section.sql`:
```sql
-- name: UpsertSection :exec
INSERT INTO section (code, label, active, display_order)
VALUES (?, ?, ?, ?)
ON CONFLICT(code) DO UPDATE SET
  label=excluded.label, active=excluded.active, display_order=excluded.display_order;

-- name: ListSections :many
SELECT code, label, active, display_order FROM section ORDER BY display_order, code;

-- name: AddPartnerSection :exec
INSERT INTO partner_section (partner_id, section_code) VALUES (?, ?)
ON CONFLICT(partner_id, section_code) DO NOTHING;

-- name: ListPartnerSectionsByPartner :many
SELECT partner_id, section_code FROM partner_section WHERE partner_id = ? ORDER BY section_code;
```

Run: `make sqlc-generate`
Expected: adds Section row struct + query methods.

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/section_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestSectionRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	repo := persistence.NewSectionRepository(q)
	ctx := context.Background()

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ram, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	if err := repo.Save(ctx, oliva); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, ram); err != nil {
		t.Fatal(err)
	}

	secs, err := repo.List(ctx)
	if err != nil || len(secs) != 2 || secs[0].Code() != "oliva" {
		t.Fatalf("List = (%+v, %v)", secs, err)
	}
	if secs[0].Label() != "Secció d'oliva" {
		t.Errorf("label round trip wrong: %q", secs[0].Label())
	}
}

func TestSectionRepository_Membership(t *testing.T) {
	q := openTestDB(t)
	secRepo := persistence.NewSectionRepository(q)
	partnerRepo := persistence.NewPartnerRepository(q)
	ctx := context.Background()

	// FK requires partner + section to exist first.
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = secRepo.Save(ctx, oliva)
	p, _ := model.NewPartner(1, "Pau", "B", "X", "p@e.cat", "6", model.Productor, 1, model.NewPartnerStub(), false)
	_ = partnerRepo.Save(ctx, p)

	m, _ := model.NewPartnerSection(1, "oliva")
	if err := secRepo.AddMembership(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := secRepo.ListMembershipsByPartner(ctx, 1)
	if err != nil || len(got) != 1 || got[0].SectionCode() != "oliva" {
		t.Fatalf("memberships = (%+v, %v)", got, err)
	}
}
```

Note: replace `model.NewPartnerStub()` with a real `time.Time` (e.g. `time.Date(2023,4,21,0,0,0,0,time.UTC)`) and add the `time` import — there is no such helper. (Written this way to remind you the partner's `addedOn` is a `time.Time`.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestSectionRepository -v`
Expected: FAIL — undefined `NewSectionRepository`.

- [ ] **Step 4: Write the mapper**

Create `internal/adapters/persistence/mapper/section.go`:
```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func SectionToRow(s model.Section) sqlc.UpsertSectionParams {
	active := int64(0)
	if s.Active() {
		active = 1
	}
	return sqlc.UpsertSectionParams{
		Code:         s.Code(),
		Label:        s.Label(),
		Active:       active,
		DisplayOrder: int64(s.DisplayOrder()),
	}
}

func SectionFromRow(r sqlc.Section) (model.Section, error) {
	return model.NewSection(r.Code, r.Label, r.Active == 1, int(r.DisplayOrder))
}

func PartnerSectionFromRow(r sqlc.PartnerSection) (model.PartnerSection, error) {
	return model.NewPartnerSection(int(r.PartnerID), r.SectionCode)
}
```

- [ ] **Step 5: Write the repository**

Create `internal/adapters/persistence/section_repository.go`:
```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type SectionRepository struct {
	q *sqlc.Queries
}

func NewSectionRepository(q *sqlc.Queries) *SectionRepository {
	return &SectionRepository{q: q}
}

func (r *SectionRepository) Save(ctx context.Context, s model.Section) error {
	return r.q.UpsertSection(ctx, mapper.SectionToRow(s))
}

func (r *SectionRepository) List(ctx context.Context) ([]model.Section, error) {
	rows, err := r.q.ListSections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Section, 0, len(rows))
	for _, row := range rows {
		s, err := mapper.SectionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *SectionRepository) AddMembership(ctx context.Context, m model.PartnerSection) error {
	return r.q.AddPartnerSection(ctx, sqlc.AddPartnerSectionParams{
		PartnerID:   int64(m.PartnerID()),
		SectionCode: m.SectionCode(),
	})
}

func (r *SectionRepository) ListMembershipsByPartner(ctx context.Context, partnerID int) ([]model.PartnerSection, error) {
	rows, err := r.q.ListPartnerSectionsByPartner(ctx, int64(partnerID))
	if err != nil {
		return nil, err
	}
	out := make([]model.PartnerSection, 0, len(rows))
	for _, row := range rows {
		m, err := mapper.PartnerSectionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/queries/section.sql internal/adapters/persistence/
git commit -m "feat(persistence): section repository with partner-section membership"
```

---

### Task 10: Taxonomy repository

**Files:**
- Create: `db/queries/taxonomy.sql`
- Create: `internal/adapters/persistence/mapper/taxonomy.go`
- Create: `internal/adapters/persistence/taxonomy_repository.go`
- Test: `internal/adapters/persistence/taxonomy_repository_test.go`

**Interfaces:**
- Produces: `persistence.NewTaxonomyRepository(q *sqlc.Queries) *TaxonomyRepository` implementing `ports.TaxonomyRepository`.
- Note: types/subtypes FK to `submission_window(year)` and `expense_type(year,code)`, so tests must insert a window + type before a subtype.

- [ ] **Step 1: Write queries and generate**

Create `db/queries/taxonomy.sql`:
```sql
-- name: UpsertExpenseType :exec
INSERT INTO expense_type (year, code, label, category)
VALUES (?, ?, ?, ?)
ON CONFLICT(year, code) DO UPDATE SET label=excluded.label, category=excluded.category;

-- name: UpsertExpenseSubtype :exec
INSERT INTO expense_subtype (year, code, label, type_code)
VALUES (?, ?, ?, ?)
ON CONFLICT(year, code) DO UPDATE SET label=excluded.label, type_code=excluded.type_code;

-- name: ListExpenseTypes :many
SELECT year, code, label, category FROM expense_type WHERE year = ? ORDER BY code;

-- name: ListExpenseSubtypes :many
SELECT year, code, label, type_code FROM expense_subtype WHERE year = ? ORDER BY code;
```

Run: `make sqlc-generate`

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/taxonomy_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func seedWindow2026(t *testing.T, q interface{ /* placeholder */ }) {}

func TestTaxonomyRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	winRepo := persistence.NewWindowRepository(q)
	taxRepo := persistence.NewTaxonomyRepository(q)
	ctx := context.Background()

	// window FK first
	w, _ := model.NewSubmissionWindow(2026, model.WindowDraft, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := winRepo.Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	typ, _ := model.NewExpenseType(2026, "A", "[a] Despeses corrents", model.CategoryCurrent)
	if err := taxRepo.SaveType(ctx, typ); err != nil {
		t.Fatal(err)
	}
	// a2/a3 share a label but are distinct codes (opaque-code quirk).
	st2, _ := model.NewExpenseSubtype(2026, "a2", "[a2] Activitats d'informació", "A")
	st3, _ := model.NewExpenseSubtype(2026, "a3", "[a2] Activitats d'informació", "A")
	if err := taxRepo.SaveSubtype(ctx, st2); err != nil {
		t.Fatal(err)
	}
	if err := taxRepo.SaveSubtype(ctx, st3); err != nil {
		t.Fatal(err)
	}

	types, err := taxRepo.ListTypes(ctx, 2026)
	if err != nil || len(types) != 1 || types[0].Category() != model.CategoryCurrent {
		t.Fatalf("ListTypes = (%+v, %v)", types, err)
	}
	subs, err := taxRepo.ListSubtypes(ctx, 2026)
	if err != nil || len(subs) != 2 {
		t.Fatalf("ListSubtypes len=%d err=%v", len(subs), err)
	}
	if subs[0].Code() == subs[1].Code() {
		t.Error("a2 and a3 must remain distinct codes")
	}
}
```

(Delete the unused `seedWindow2026` stub before committing; it is only here to flag that the FK ordering matters.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestTaxonomyRepository -v`
Expected: FAIL — undefined `NewTaxonomyRepository` (and `NewWindowRepository` if Task 11 not yet done; if so, do Task 11 first or stub the window via raw SQL — but the intended order is 10 then 11; to keep this task self-contained, insert the window with a direct `q.UpsertSubmissionWindow` once Task 11's query exists. If you reach this task before Task 11, swap the order.)

- [ ] **Step 4: Write the mapper**

Create `internal/adapters/persistence/mapper/taxonomy.go`:
```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ExpenseTypeToRow(t model.ExpenseType) sqlc.UpsertExpenseTypeParams {
	return sqlc.UpsertExpenseTypeParams{
		Year:     int64(t.Year()),
		Code:     t.Code(),
		Label:    t.Label(),
		Category: string(t.Category()),
	}
}

func ExpenseTypeFromRow(r sqlc.ExpenseType) (model.ExpenseType, error) {
	cat, err := model.ParseExpenseCategory(r.Category)
	if err != nil {
		return model.ExpenseType{}, err
	}
	return model.NewExpenseType(int(r.Year), r.Code, r.Label, cat)
}

func ExpenseSubtypeToRow(s model.ExpenseSubtype) sqlc.UpsertExpenseSubtypeParams {
	return sqlc.UpsertExpenseSubtypeParams{
		Year:     int64(s.Year()),
		Code:     s.Code(),
		Label:    s.Label(),
		TypeCode: s.TypeCode(),
	}
}

func ExpenseSubtypeFromRow(r sqlc.ExpenseSubtype) (model.ExpenseSubtype, error) {
	return model.NewExpenseSubtype(int(r.Year), r.Code, r.Label, r.TypeCode)
}
```

- [ ] **Step 5: Write the repository**

Create `internal/adapters/persistence/taxonomy_repository.go`:
```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type TaxonomyRepository struct {
	q *sqlc.Queries
}

func NewTaxonomyRepository(q *sqlc.Queries) *TaxonomyRepository {
	return &TaxonomyRepository{q: q}
}

func (r *TaxonomyRepository) SaveType(ctx context.Context, t model.ExpenseType) error {
	return r.q.UpsertExpenseType(ctx, mapper.ExpenseTypeToRow(t))
}

func (r *TaxonomyRepository) SaveSubtype(ctx context.Context, s model.ExpenseSubtype) error {
	return r.q.UpsertExpenseSubtype(ctx, mapper.ExpenseSubtypeToRow(s))
}

func (r *TaxonomyRepository) ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error) {
	rows, err := r.q.ListExpenseTypes(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseType, 0, len(rows))
	for _, row := range rows {
		t, err := mapper.ExpenseTypeFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *TaxonomyRepository) ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error) {
	rows, err := r.q.ListExpenseSubtypes(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseSubtype, 0, len(rows))
	for _, row := range rows {
		s, err := mapper.ExpenseSubtypeFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/queries/taxonomy.sql internal/adapters/persistence/
git commit -m "feat(persistence): taxonomy repository (types + subtypes)"
```

---

### Task 11: Window repository

**Files:**
- Create: `db/queries/window.sql`
- Create: `internal/adapters/persistence/mapper/window.go`
- Create: `internal/adapters/persistence/window_repository.go`
- Test: `internal/adapters/persistence/window_repository_test.go`

**Interfaces:**
- Produces: `persistence.NewWindowRepository(q *sqlc.Queries) *WindowRepository` implementing `ports.WindowRepository`. (Already referenced by Task 10's test — implement this before or alongside Task 10.)

- [ ] **Step 1: Write queries and generate**

Create `db/queries/window.sql`:
```sql
-- name: UpsertSubmissionWindow :exec
INSERT INTO submission_window (year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(year) DO UPDATE SET
  state=excluded.state, opened_at=excluded.opened_at, closed_at=excluded.closed_at,
  deadline=excluded.deadline, current_expense_limit=excluded.current_expense_limit,
  investment_expense_limit=excluded.investment_expense_limit;

-- name: GetSubmissionWindow :one
SELECT year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit
FROM submission_window WHERE year = ?;

-- name: ListSubmissionWindows :many
SELECT year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit
FROM submission_window ORDER BY year DESC;
```

Run: `make sqlc-generate`

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/window_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestWindowRepository_RoundTrip(t *testing.T) {
	repo := persistence.NewWindowRepository(openTestDB(t))
	ctx := context.Background()

	deadline := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	w, _ := model.NewSubmissionWindow(2026, model.WindowClosed, nil, nil, deadline,
		model.MoneyOf(30000), model.MoneyOf(70000))
	if err := repo.Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	got, found, err := repo.FindByYear(ctx, 2026)
	if err != nil || !found {
		t.Fatalf("FindByYear: (%v, %v)", found, err)
	}
	if got.State() != model.WindowClosed {
		t.Errorf("state = %q", got.State())
	}
	if got.CurrentExpenseLimit().String() != "30000.00" {
		t.Errorf("current limit = %q, want 30000.00", got.CurrentExpenseLimit().String())
	}
	if !got.Deadline().Equal(deadline) {
		t.Errorf("deadline = %v, want %v", got.Deadline(), deadline)
	}
	if got.OpenedAt() != nil {
		t.Errorf("OpenedAt should be nil, got %v", got.OpenedAt())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestWindowRepository -v`
Expected: FAIL — undefined `NewWindowRepository`.

- [ ] **Step 4: Write the mapper**

Create `internal/adapters/persistence/mapper/window.go`:
```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func WindowToRow(w model.SubmissionWindow) sqlc.UpsertSubmissionWindowParams {
	return sqlc.UpsertSubmissionWindowParams{
		Year:                   int64(w.Year()),
		State:                  string(w.State()),
		OpenedAt:               FormatNullableTimestamp(w.OpenedAt()),
		ClosedAt:               FormatNullableTimestamp(w.ClosedAt()),
		Deadline:               FormatTimestamp(w.Deadline()),
		CurrentExpenseLimit:    w.CurrentExpenseLimit().String(),
		InvestmentExpenseLimit: w.InvestmentExpenseLimit().String(),
	}
}

func WindowFromRow(r sqlc.SubmissionWindow) (model.SubmissionWindow, error) {
	state, err := model.ParseWindowState(r.State)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	openedAt, err := ParseNullableTimestamp(r.OpenedAt)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	closedAt, err := ParseNullableTimestamp(r.ClosedAt)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	deadline, err := ParseTimestamp(r.Deadline)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	current, err := model.MoneyFromString(r.CurrentExpenseLimit)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	investment, err := model.MoneyFromString(r.InvestmentExpenseLimit)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	return model.NewSubmissionWindow(int(r.Year), state, openedAt, closedAt, deadline, current, investment)
}
```

- [ ] **Step 5: Write the repository**

Create `internal/adapters/persistence/window_repository.go`:
```go
package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type WindowRepository struct {
	q *sqlc.Queries
}

func NewWindowRepository(q *sqlc.Queries) *WindowRepository {
	return &WindowRepository{q: q}
}

func (r *WindowRepository) Save(ctx context.Context, w model.SubmissionWindow) error {
	return r.q.UpsertSubmissionWindow(ctx, mapper.WindowToRow(w))
}

func (r *WindowRepository) FindByYear(ctx context.Context, year int) (model.SubmissionWindow, bool, error) {
	row, err := r.q.GetSubmissionWindow(ctx, int64(year))
	if errors.Is(err, sql.ErrNoRows) {
		return model.SubmissionWindow{}, false, nil
	}
	if err != nil {
		return model.SubmissionWindow{}, false, err
	}
	w, err := mapper.WindowFromRow(row)
	return w, err == nil, err
}

func (r *WindowRepository) List(ctx context.Context) ([]model.SubmissionWindow, error) {
	rows, err := r.q.ListSubmissionWindows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.SubmissionWindow, 0, len(rows))
	for _, row := range rows {
		w, err := mapper.WindowFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS (window + taxonomy now both green).

- [ ] **Step 7: Commit**

```bash
git add db/queries/window.sql internal/adapters/persistence/
git commit -m "feat(persistence): submission window repository"
```

---

### Task 12: Forecast repository (with next-id allocation)

**Files:**
- Create: `db/queries/forecast.sql`
- Create: `internal/adapters/persistence/mapper/forecast.go`
- Create: `internal/adapters/persistence/forecast_repository.go`
- Test: `internal/adapters/persistence/forecast_repository_test.go`

**Interfaces:**
- Consumes: `forecastid.Format`, `forecastid.ParseSeq`, `model.ExpenseForecast`, `ports.ForecastRepository`.
- Produces: `persistence.NewForecastRepository(conn *sql.DB, q *sqlc.Queries) *ForecastRepository` implementing `ports.ForecastRepository`. It needs the `*sql.DB` to run next-id allocation + insert in one transaction.

- [ ] **Step 1: Write queries and generate**

Create `db/queries/forecast.sql`:
```sql
-- name: InsertForecast :exec
INSERT INTO expense_forecast
  (id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
   planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateForecast :exec
UPDATE expense_forecast SET
  partner_id=?, concept=?, description=?, gross_amount=?, approved_amount=?, approved_on=?,
  planned_date=?, year=?, subtype_code=?, scope_kind=?, section_code=?, added_on=?, enabled=?
WHERE id=?;

-- name: GetForecast :one
SELECT id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
       planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled
FROM expense_forecast WHERE id = ?;

-- name: ListForecastsByYear :many
SELECT id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
       planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled
FROM expense_forecast WHERE year = ? ORDER BY id;

-- name: ListForecastIDsByYear :many
SELECT id FROM expense_forecast WHERE year = ?;
```

Run: `make sqlc-generate`

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/forecast_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedForYear inserts the window + a type + subtype + partner the forecast FKs require.
func seedForYear(t *testing.T, q *sqlc.Queries, year int) {
	t.Helper()
	ctx := context.Background()
	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	w, _ := model.NewSubmissionWindow(year, model.WindowOpen, nil, nil,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := win.Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	typ, _ := model.NewExpenseType(year, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(year, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, st)
	p, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p)
}

func newForecastRepo(t *testing.T) (*persistence.ForecastRepository, *sqlc.Queries) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	return persistence.NewForecastRepository(conn, q), q
}

func TestForecastRepository_CreateAllocatesIDAndRoundTrips(t *testing.T) {
	repo, q := newForecastRepo(t)
	seedForYear(t, q, 2026)
	ctx := context.Background()

	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	f, _ := model.NewExpenseForecast("", 7, "Concepte", "desc", model.MoneyOf(2880), model.ZeroMoney(),
		nil, planned, 2026, "a1", model.NewCommonScope(), planned, true)

	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID() != "CP26001" {
		t.Errorf("first id = %q, want CP26001", created.ID())
	}

	second, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID() != "CP26002" {
		t.Errorf("second id = %q, want CP26002", second.ID())
	}

	got, found, err := repo.FindByID(ctx, "CP26001")
	if err != nil || !found {
		t.Fatalf("FindByID: (%v, %v)", found, err)
	}
	if got.GrossAmount().String() != "2880.00" || got.Scope().Kind() != model.ScopeCommon {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestForecastRepository_SectionScopeAndMoneyExactness(t *testing.T) {
	repo, q := newForecastRepo(t)
	seedForYear(t, q, 2026)
	ctx := context.Background()
	// section FK
	secRepo := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = secRepo.Save(ctx, oliva)

	planned := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	sectionScope, _ := model.NewSectionScope("oliva")
	gross, _ := model.MoneyFromString("1322.22") // the former-REAL value
	f, _ := model.NewExpenseForecast("", 7, "C", "d", gross, model.ZeroMoney(),
		nil, planned, 2026, "a1", sectionScope, planned, true)

	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := repo.FindByID(ctx, created.ID())
	if got.GrossAmount().String() != "1322.22" {
		t.Errorf("money exactness lost: %q", got.GrossAmount().String())
	}
	if got.Scope().Kind() != model.ScopeSection || got.Scope().SectionCode() != "oliva" {
		t.Errorf("section scope round trip wrong: %+v", got.Scope())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestForecastRepository -v`
Expected: FAIL — undefined `NewForecastRepository`.

- [ ] **Step 4: Write the mapper**

Create `internal/adapters/persistence/mapper/forecast.go`:
```go
package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullableSection(s model.ExpenseScope) sql.NullString {
	if s.Kind() == model.ScopeSection {
		return sql.NullString{String: s.SectionCode(), Valid: true}
	}
	return sql.NullString{}
}

func ForecastToInsert(f model.ExpenseForecast) sqlc.InsertForecastParams {
	return sqlc.InsertForecastParams{
		ID:             f.ID(),
		PartnerID:      int64(f.PartnerID()),
		Concept:        f.Concept(),
		Description:    f.Description(),
		GrossAmount:    f.GrossAmount().String(),
		ApprovedAmount: f.ApprovedAmount().String(),
		ApprovedOn:     FormatNullableTimestamp(f.ApprovedOn()),
		PlannedDate:    FormatDate(f.PlannedDate()),
		Year:           int64(f.Year()),
		SubtypeCode:    f.SubtypeCode(),
		ScopeKind:      string(f.Scope().Kind()),
		SectionCode:    nullableSection(f.Scope()),
		AddedOn:        FormatTimestamp(f.AddedOn()),
		Enabled:        boolToInt(f.Enabled()),
	}
}

func ForecastToUpdate(f model.ExpenseForecast) sqlc.UpdateForecastParams {
	return sqlc.UpdateForecastParams{
		ID:             f.ID(),
		PartnerID:      int64(f.PartnerID()),
		Concept:        f.Concept(),
		Description:    f.Description(),
		GrossAmount:    f.GrossAmount().String(),
		ApprovedAmount: f.ApprovedAmount().String(),
		ApprovedOn:     FormatNullableTimestamp(f.ApprovedOn()),
		PlannedDate:    FormatDate(f.PlannedDate()),
		Year:           int64(f.Year()),
		SubtypeCode:    f.SubtypeCode(),
		ScopeKind:      string(f.Scope().Kind()),
		SectionCode:    nullableSection(f.Scope()),
		AddedOn:        FormatTimestamp(f.AddedOn()),
		Enabled:        boolToInt(f.Enabled()),
	}
}

func ForecastFromRow(r sqlc.ExpenseForecast) (model.ExpenseForecast, error) {
	gross, err := model.MoneyFromString(r.GrossAmount)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	approved, err := model.MoneyFromString(r.ApprovedAmount)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	approvedOn, err := ParseNullableTimestamp(r.ApprovedOn)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	planned, err := ParseDate(r.PlannedDate)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	added, err := ParseTimestamp(r.AddedOn)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	kind, err := model.ParseScopeKind(r.ScopeKind)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	sectionCode := ""
	if r.SectionCode.Valid {
		sectionCode = r.SectionCode.String
	}
	scope, err := model.NewScope(kind, sectionCode)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	return model.NewExpenseForecast(r.ID, int(r.PartnerID), r.Concept, r.Description,
		gross, approved, approvedOn, planned, int(r.Year), r.SubtypeCode, scope, added, r.Enabled == 1)
}
```

- [ ] **Step 5: Write the repository (with transactional next-id)**

Create `internal/adapters/persistence/forecast_repository.go`:
```go
package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/forecastid"
	"github.com/pjover/espigol/internal/domain/model"
)

type ForecastRepository struct {
	conn *sql.DB
	q    *sqlc.Queries
}

func NewForecastRepository(conn *sql.DB, q *sqlc.Queries) *ForecastRepository {
	return &ForecastRepository{conn: conn, q: q}
}

// Create allocates the next CPYYnnn id for the forecast's year and inserts it,
// within a single transaction so concurrent creates cannot collide.
func (r *ForecastRepository) Create(ctx context.Context, f model.ExpenseForecast) (model.ExpenseForecast, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	defer tx.Rollback()
	qtx := r.q.WithTx(tx)

	ids, err := qtx.ListForecastIDsByYear(ctx, int64(f.Year()))
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	maxSeq := -1
	for _, id := range ids {
		_, seq, err := forecastid.ParseSeq(id)
		if err != nil {
			return model.ExpenseForecast{}, err
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	newID, err := forecastid.Format(f.Year(), maxSeq+1)
	if err != nil {
		return model.ExpenseForecast{}, err
	}

	withID, err := rebuildWithID(f, newID)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	if err := qtx.InsertForecast(ctx, mapper.ForecastToInsert(withID)); err != nil {
		return model.ExpenseForecast{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ExpenseForecast{}, err
	}
	return withID, nil
}

func (r *ForecastRepository) Save(ctx context.Context, f model.ExpenseForecast) error {
	return r.q.UpdateForecast(ctx, mapper.ForecastToUpdate(f))
}

func (r *ForecastRepository) FindByID(ctx context.Context, id string) (model.ExpenseForecast, bool, error) {
	row, err := r.q.GetForecast(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ExpenseForecast{}, false, nil
	}
	if err != nil {
		return model.ExpenseForecast{}, false, err
	}
	f, err := mapper.ForecastFromRow(row)
	return f, err == nil, err
}

func (r *ForecastRepository) ListByYear(ctx context.Context, year int) ([]model.ExpenseForecast, error) {
	rows, err := r.q.ListForecastsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseForecast, 0, len(rows))
	for _, row := range rows {
		f, err := mapper.ForecastFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// rebuildWithID returns a copy of f with the given id, re-running domain validation.
func rebuildWithID(f model.ExpenseForecast, id string) (model.ExpenseForecast, error) {
	return model.NewExpenseForecast(id, f.PartnerID(), f.Concept(), f.Description(),
		f.GrossAmount(), f.ApprovedAmount(), f.ApprovedOn(), f.PlannedDate(), f.Year(),
		f.SubtypeCode(), f.Scope(), f.AddedOn(), f.Enabled())
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/queries/forecast.sql internal/adapters/persistence/
git commit -m "feat(persistence): forecast repository with transactional CPYYnnn id allocation"
```

---

### Task 13: Report repository

**Files:**
- Create: `db/queries/report.sql`
- Create: `internal/adapters/persistence/mapper/report.go`
- Create: `internal/adapters/persistence/report_repository.go`
- Test: `internal/adapters/persistence/report_repository_test.go`

**Interfaces:**
- Produces: `persistence.NewReportRepository(q *sqlc.Queries) *ReportRepository` implementing `ports.ReportRepository` (`Insert` returns the new id; `FindLatestByYear` returns the non-superseded latest; `MarkSuperseded`).

- [ ] **Step 1: Write queries and generate**

Create `db/queries/report.sql`:
```sql
-- name: InsertReport :one
INSERT INTO report (year, generated_at, snapshot_json, pdf, superseded_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetLatestReportByYear :one
SELECT id, year, generated_at, snapshot_json, pdf, superseded_at
FROM report
WHERE year = ? AND superseded_at IS NULL
ORDER BY generated_at DESC
LIMIT 1;

-- name: MarkReportSuperseded :exec
UPDATE report SET superseded_at = ? WHERE id = ?;
```

Run: `make sqlc-generate`

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/report_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportRepository_InsertAndLatest(t *testing.T) {
	q := openTestDB(t)
	winRepo := persistence.NewWindowRepository(q)
	repo := persistence.NewReportRepository(q)
	ctx := context.Background()

	w, _ := model.NewSubmissionWindow(2026, model.WindowClosed, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = winRepo.Save(ctx, w)

	gen := time.Date(2026, 6, 23, 15, 15, 59, 0, time.UTC)
	r1, _ := model.NewReport(0, 2026, gen, `{"a":1}`, []byte{0x25, 0x50}, nil)
	id1, err := repo.Insert(ctx, r1)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, found, err := repo.FindLatestByYear(ctx, 2026)
	if err != nil || !found || got.ID() != id1 {
		t.Fatalf("FindLatest: (%+v, %v, %v)", got, found, err)
	}
	if got.SnapshotJSON() != `{"a":1}` || len(got.Pdf()) != 2 {
		t.Errorf("round trip mismatch: json=%q pdfLen=%d", got.SnapshotJSON(), len(got.Pdf()))
	}

	// supersede r1, insert r2 -> latest is r2
	if err := repo.MarkSuperseded(ctx, id1, gen.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r2, _ := model.NewReport(0, 2026, gen.Add(2*time.Hour), `{"a":2}`, []byte{0x25}, nil)
	id2, _ := repo.Insert(ctx, r2)
	latest, _, _ := repo.FindLatestByYear(ctx, 2026)
	if latest.ID() != id2 {
		t.Errorf("latest id = %d, want %d", latest.ID(), id2)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestReportRepository -v`
Expected: FAIL — undefined `NewReportRepository`.

- [ ] **Step 4: Write the mapper**

Create `internal/adapters/persistence/mapper/report.go`:
```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ReportToInsert(r model.Report) sqlc.InsertReportParams {
	return sqlc.InsertReportParams{
		Year:         int64(r.Year()),
		GeneratedAt:  FormatTimestamp(r.GeneratedAt()),
		SnapshotJson: r.SnapshotJSON(),
		Pdf:          r.Pdf(),
		SupersededAt: FormatNullableTimestamp(r.SupersededAt()),
	}
}

func ReportFromRow(r sqlc.Report) (model.Report, error) {
	generatedAt, err := ParseTimestamp(r.GeneratedAt)
	if err != nil {
		return model.Report{}, err
	}
	supersededAt, err := ParseNullableTimestamp(r.SupersededAt)
	if err != nil {
		return model.Report{}, err
	}
	return model.NewReport(int(r.ID), int(r.Year), generatedAt, r.SnapshotJson, r.Pdf, supersededAt)
}
```

(Field name `SnapshotJson` follows sqlc's default capitalization of `snapshot_json`; confirm against the generated struct and adjust if needed.)

- [ ] **Step 5: Write the repository**

Create `internal/adapters/persistence/report_repository.go`:
```go
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type ReportRepository struct {
	q *sqlc.Queries
}

func NewReportRepository(q *sqlc.Queries) *ReportRepository {
	return &ReportRepository{q: q}
}

func (r *ReportRepository) Insert(ctx context.Context, rep model.Report) (int, error) {
	id, err := r.q.InsertReport(ctx, mapper.ReportToInsert(rep))
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *ReportRepository) FindLatestByYear(ctx context.Context, year int) (model.Report, bool, error) {
	row, err := r.q.GetLatestReportByYear(ctx, int64(year))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Report{}, false, nil
	}
	if err != nil {
		return model.Report{}, false, err
	}
	rep, err := mapper.ReportFromRow(row)
	return rep, err == nil, err
}

func (r *ReportRepository) MarkSuperseded(ctx context.Context, id int, at time.Time) error {
	return r.q.MarkReportSuperseded(ctx, sqlc.MarkReportSupersededParams{
		SupersededAt: mapper.FormatNullableTimestamp(&at),
		ID:           int64(id),
	})
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/adapters/persistence/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/queries/report.sql internal/adapters/persistence/
git commit -m "feat(persistence): report repository with supersede semantics"
```

---

### Task 14: Audit + BoardAuthorization repositories

**Files:**
- Create: `db/queries/audit.sql`, `db/queries/board.sql`
- Create: `internal/adapters/persistence/mapper/audit.go`, `mapper/board.go`
- Create: `internal/adapters/persistence/audit_repository.go`, `board_repository.go`
- Test: `internal/adapters/persistence/audit_repository_test.go`, `board_repository_test.go`

**Interfaces:**
- Produces: `persistence.NewAuditLog(q *sqlc.Queries) *AuditLog` (impl `ports.AuditLog`) and `persistence.NewBoardAuthorizationRepository(q *sqlc.Queries) *BoardAuthorizationRepository` (impl `ports.BoardAuthorizationRepository`).

- [ ] **Step 1: Write queries and generate**

Create `db/queries/audit.sql`:
```sql
-- name: InsertAuditEvent :exec
INSERT INTO audit_event (actor_id, actor_email, kind, entity_type, entity_id, timestamp, payload)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT id, actor_id, actor_email, kind, entity_type, entity_id, timestamp, payload
FROM audit_event ORDER BY id;
```

Create `db/queries/board.sql`:
```sql
-- name: UpsertBoardAuthorization :exec
INSERT INTO board_authorization (partner_id, scope_kind, section_code)
VALUES (?, ?, ?)
ON CONFLICT(partner_id, scope_kind, COALESCE(section_code, '')) DO NOTHING;

-- name: ListBoardAuthorizationsByPartner :many
SELECT partner_id, scope_kind, section_code
FROM board_authorization WHERE partner_id = ? ORDER BY scope_kind, section_code;
```

Run: `make sqlc-generate`

- [ ] **Step 2: Write the failing tests**

Create `internal/adapters/persistence/audit_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestAuditLog_AppendAndList(t *testing.T) {
	repo := persistence.NewAuditLog(openTestDB(t))
	ctx := context.Background()

	payload := `{"imported":1}`
	e, _ := model.NewAuditEvent(0, nil, "system@espigol", model.AuditMigration,
		"Partner", "1", time.Date(2026, 6, 23, 15, 15, 59, 0, time.UTC), &payload)
	if err := repo.Append(ctx, e); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("List: len=%d err=%v", len(all), err)
	}
	if all[0].ActorID() != nil {
		t.Errorf("ActorID should be nil, got %v", all[0].ActorID())
	}
	if all[0].Payload() == nil || *all[0].Payload() != payload {
		t.Errorf("payload round trip wrong: %v", all[0].Payload())
	}
}
```

Create `internal/adapters/persistence/board_repository_test.go`:
```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestBoardAuthorizationRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	repo := persistence.NewBoardAuthorizationRepository(q)
	ctx := context.Background()

	p, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), true)
	_ = pr.Save(ctx, p)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)

	common, _ := model.NewBoardAuthorization(7, model.ScopeCommon, "")
	section, _ := model.NewBoardAuthorization(7, model.ScopeSection, "oliva")
	if err := repo.Save(ctx, common); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, section); err != nil {
		t.Fatal(err)
	}
	// idempotent: saving common again must not error or duplicate.
	if err := repo.Save(ctx, common); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByPartner(ctx, 7)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByPartner: len=%d err=%v", len(got), err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapters/persistence/ -run 'TestAuditLog|TestBoardAuthorization' -v`
Expected: FAIL — undefined constructors.

- [ ] **Step 4: Write the mappers**

Create `internal/adapters/persistence/mapper/audit.go`:
```go
package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func nullableInt(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullableString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func AuditToInsert(e model.AuditEvent) sqlc.InsertAuditEventParams {
	return sqlc.InsertAuditEventParams{
		ActorID:    nullableInt(e.ActorID()),
		ActorEmail: e.ActorEmail(),
		Kind:       string(e.Kind()),
		EntityType: e.EntityType(),
		EntityID:   e.EntityID(),
		Timestamp:  FormatTimestamp(e.Timestamp()),
		Payload:    nullableString(e.Payload()),
	}
}

func AuditFromRow(r sqlc.AuditEvent) (model.AuditEvent, error) {
	ts, err := ParseTimestamp(r.Timestamp)
	if err != nil {
		return model.AuditEvent{}, err
	}
	kind, err := model.ParseAuditKind(r.Kind)
	if err != nil {
		return model.AuditEvent{}, err
	}
	var actorID *int
	if r.ActorID.Valid {
		v := int(r.ActorID.Int64)
		actorID = &v
	}
	var payload *string
	if r.Payload.Valid {
		payload = &r.Payload.String
	}
	return model.NewAuditEvent(int(r.ID), actorID, r.ActorEmail, kind,
		r.EntityType, r.EntityID, ts, payload)
}
```

Create `internal/adapters/persistence/mapper/board.go`:
```go
package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func BoardAuthToRow(a model.BoardAuthorization) sqlc.UpsertBoardAuthorizationParams {
	section := sql.NullString{}
	if a.SectionCode() != "" {
		section = sql.NullString{String: a.SectionCode(), Valid: true}
	}
	return sqlc.UpsertBoardAuthorizationParams{
		PartnerID:   int64(a.PartnerID()),
		ScopeKind:   string(a.ScopeKind()),
		SectionCode: section,
	}
}

func BoardAuthFromRow(r sqlc.BoardAuthorization) (model.BoardAuthorization, error) {
	kind, err := model.ParseScopeKind(r.ScopeKind)
	if err != nil {
		return model.BoardAuthorization{}, err
	}
	section := ""
	if r.SectionCode.Valid {
		section = r.SectionCode.String
	}
	return model.NewBoardAuthorization(int(r.PartnerID), kind, section)
}
```

- [ ] **Step 5: Write the repositories**

Create `internal/adapters/persistence/audit_repository.go`:
```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type AuditLog struct {
	q *sqlc.Queries
}

func NewAuditLog(q *sqlc.Queries) *AuditLog {
	return &AuditLog{q: q}
}

func (a *AuditLog) Append(ctx context.Context, e model.AuditEvent) error {
	return a.q.InsertAuditEvent(ctx, mapper.AuditToInsert(e))
}

func (a *AuditLog) List(ctx context.Context) ([]model.AuditEvent, error) {
	rows, err := a.q.ListAuditEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.AuditEvent, 0, len(rows))
	for _, row := range rows {
		e, err := mapper.AuditFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
```

Create `internal/adapters/persistence/board_repository.go`:
```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type BoardAuthorizationRepository struct {
	q *sqlc.Queries
}

func NewBoardAuthorizationRepository(q *sqlc.Queries) *BoardAuthorizationRepository {
	return &BoardAuthorizationRepository{q: q}
}

func (r *BoardAuthorizationRepository) Save(ctx context.Context, a model.BoardAuthorization) error {
	return r.q.UpsertBoardAuthorization(ctx, mapper.BoardAuthToRow(a))
}

func (r *BoardAuthorizationRepository) ListByPartner(ctx context.Context, partnerID int) ([]model.BoardAuthorization, error) {
	rows, err := r.q.ListBoardAuthorizationsByPartner(ctx, int64(partnerID))
	if err != nil {
		return nil, err
	}
	out := make([]model.BoardAuthorization, 0, len(rows))
	for _, row := range rows {
		a, err := mapper.BoardAuthFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
```

- [ ] **Step 6: Add port compile-time checks**

Create `internal/adapters/persistence/ports_check.go`:
```go
package persistence

import "github.com/pjover/espigol/internal/domain/ports"

var (
	_ ports.PartnerRepository            = (*PartnerRepository)(nil)
	_ ports.SectionRepository            = (*SectionRepository)(nil)
	_ ports.TaxonomyRepository           = (*TaxonomyRepository)(nil)
	_ ports.WindowRepository             = (*WindowRepository)(nil)
	_ ports.ForecastRepository           = (*ForecastRepository)(nil)
	_ ports.ReportRepository             = (*ReportRepository)(nil)
	_ ports.AuditLog                     = (*AuditLog)(nil)
	_ ports.BoardAuthorizationRepository = (*BoardAuthorizationRepository)(nil)
)
```

(If any assertion fails to compile, the repository's method set diverged from its port — reconcile names/signatures with `internal/domain/ports/ports.go`. Remove the optional ad-hoc check left in `partner_repository.go` in Task 8 in favor of this file.)

- [ ] **Step 7: Run all persistence tests + build**

Run: `go test ./internal/adapters/persistence/... -v && go build ./...`
Expected: PASS, build clean (all ports satisfied).

- [ ] **Step 8: Commit**

```bash
git add db/queries/audit.sql db/queries/board.sql internal/adapters/persistence/
git commit -m "feat(persistence): audit log + board authorization repositories"
```

---

### Task 15: Adopt tool — legacy reader

**Files:**
- Create: `cmd/adopt/legacy/legacy.go` (read the old Java schema into plain structs)
- Test: `cmd/adopt/legacy/legacy_test.go`
- Create: `testdata/.gitignore` (ignore the DB snapshot) and copy the fixture (manual step)

**Interfaces:**
- Produces: `legacy.Read(path string) (*legacy.Dump, error)` returning plain structs:
  ```go
  type Dump struct {
      Partners  []Partner
      Sections  []string // implied; derived in transform, not here
      Types     []ExpenseType
      Subtypes  []ExpenseSubtype
      Windows   []SubmissionWindow
      Forecasts []ExpenseForecast
      Reports   []Report
      Audits    []AuditEvent
  }
  ```
  with legacy field shapes (e.g. `Partner{ID int; ...; OliveSection bool; LivestockSection bool}`, `ExpenseForecast{ID string; ...; GrossAmount string; ApprovedAmount string; Scope string; ...}` where money is already converted to an exact decimal string and `Scope` is the Catalan string). Times parsed from the legacy `2006-01-02 15:04:05.000` format.

- [ ] **Step 1: Place the test fixture**

Run (copies a frozen snapshot of the real Java DB into this repo, gitignored):
```bash
mkdir -p testdata
cp /home/pere/Projects/espigol-java/.local/espigol.db testdata/legacy-espigol.db
printf '*\n!.gitignore\n' > testdata/.gitignore
```
Expected: `testdata/legacy-espigol.db` exists and is ignored by git (only `testdata/.gitignore` is tracked).

- [ ] **Step 2: Write the failing reader test**

Create `cmd/adopt/legacy/legacy_test.go`:
```go
package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

const fixture = "../../../testdata/legacy-espigol.db"

func TestRead_RealFixture(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping (see testdata/)")
	}
	// Copy to a temp path so the test never mutates the fixture.
	src, _ := os.ReadFile(fixture)
	tmp := filepath.Join(t.TempDir(), "legacy.db")
	if err := os.WriteFile(tmp, src, 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(d.Partners) != 8 {
		t.Errorf("partners = %d, want 8", len(d.Partners))
	}
	if len(d.Forecasts) != 35 {
		t.Errorf("forecasts = %d, want 35", len(d.Forecasts))
	}
	if len(d.Types) != 3 || len(d.Subtypes) != 13 {
		t.Errorf("taxonomy: types=%d subtypes=%d, want 3/13", len(d.Types), len(d.Subtypes))
	}
	if len(d.Windows) != 1 || len(d.Reports) != 1 {
		t.Errorf("windows=%d reports=%d, want 1/1", len(d.Windows), len(d.Reports))
	}
	if len(d.Audits) != 61 {
		t.Errorf("audits = %d, want 61", len(d.Audits))
	}
	// money exactness: the two former-REAL values survive as strings.
	var found1322 bool
	for _, f := range d.Forecasts {
		if f.GrossAmount == "1322.22" {
			found1322 = true
		}
	}
	if !found1322 {
		t.Error("expected a forecast with gross 1322.22 read exactly")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/adopt/legacy/ -v`
Expected: FAIL — undefined `Read` (or skip if fixture absent — ensure Step 1 ran).

- [ ] **Step 4: Write the legacy reader**

Create `cmd/adopt/legacy/legacy.go`. Implement `Read(path string)` opening the DB with the modernc driver read-only and selecting every table. Key conversions:
- Money columns: select with SQLite text coercion so int/real both render exactly — use `SELECT CAST(gross_amount AS TEXT)` is **not** safe for REAL (would give `1322.22` but also e.g. `2880` for ints). Instead read the column into an `any`/`sql.RawBytes` then format: if the stored value is integer, render `"<n>.00"`; if real, render with `strconv.FormatFloat(v,'f',2,64)`. Simpler and exact: read each money column via `printf('%.2f', col)` in SQL: `SELECT printf('%.2f', gross_amount) ...` — SQLite's `printf('%.2f', x)` yields `"1322.22"` and `"2880.00"` correctly for both storage types. Use that for all money columns (forecast gross/approved, window limits).
- Times: parse `2006-01-02 15:04:05.000` (treat as UTC) via `time.Parse("2006-01-02 15:04:05.000", s)`; for the LocalDate columns (partner.added_on, forecast.planned_date) the same parse works (time component is `00:00:00.000`); the transform keeps only the date for those.
- `scope` stays the Catalan string; `olive_section`/`livestock_section` as bool.

```go
// Package legacy reads the old espigol-java SQLite schema into plain structs.
package legacy

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const legacyTimeLayout = "2006-01-02 15:04:05.000"

type Partner struct {
	ID                                       int
	Name, Surname, VatCode, Email, Mobile    string
	PartnerType                              string
	RiaNumber                                int
	OliveSection, LivestockSection           bool
	AddedOn                                  time.Time
	BoardMember                              bool
}

type ExpenseType struct{ Year int; Code, Label, Category string }
type ExpenseSubtype struct{ Year int; Code, Label, TypeCode string }

type SubmissionWindow struct {
	Year                             int
	State                            string
	OpenedAt, ClosedAt               *time.Time
	Deadline                         time.Time
	CurrentLimit, InvestmentLimit    string // exact decimal strings
}

type ExpenseForecast struct {
	ID                            string
	PartnerID                     int
	Concept, Description          string
	GrossAmount, ApprovedAmount   string // exact decimal strings
	ApprovedOn                    *time.Time
	PlannedDate                   time.Time
	Year                          int
	SubtypeCode                   string
	Scope                         string // Catalan
	AddedOn                       time.Time
	Enabled                       bool
}

type Report struct {
	ID           int
	Year         int
	GeneratedAt  time.Time
	SnapshotJSON string
	Pdf          []byte
	SupersededAt *time.Time
}

type AuditEvent struct {
	ID                                  int
	ActorID                             *int
	ActorEmail, Kind, EntityType, EntityID string
	Timestamp                           time.Time
	Payload                             *string
}

type Dump struct {
	Partners  []Partner
	Types     []ExpenseType
	Subtypes  []ExpenseSubtype
	Windows   []SubmissionWindow
	Forecasts []ExpenseForecast
	Reports   []Report
	Audits    []AuditEvent
}

func parseTime(s string) (time.Time, error) { return time.ParseInLocation(legacyTimeLayout, s, time.UTC) }

func parseNullTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Read opens the legacy DB read-only and loads every table into a Dump.
func Read(path string) (*Dump, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	d := &Dump{}
	if d.Partners, err = readPartners(conn); err != nil {
		return nil, fmt.Errorf("partners: %w", err)
	}
	if d.Types, err = readTypes(conn); err != nil {
		return nil, fmt.Errorf("types: %w", err)
	}
	if d.Subtypes, err = readSubtypes(conn); err != nil {
		return nil, fmt.Errorf("subtypes: %w", err)
	}
	if d.Windows, err = readWindows(conn); err != nil {
		return nil, fmt.Errorf("windows: %w", err)
	}
	if d.Forecasts, err = readForecasts(conn); err != nil {
		return nil, fmt.Errorf("forecasts: %w", err)
	}
	if d.Reports, err = readReports(conn); err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	if d.Audits, err = readAudits(conn); err != nil {
		return nil, fmt.Errorf("audits: %w", err)
	}
	return d, nil
}
```

Then implement each `read*` helper with explicit SQL. Use `printf('%.2f', col)` for all money columns. Example for forecasts (implement the others analogously — partners, types, subtypes, windows, reports, audits):
```go
func readForecasts(conn *sql.DB) ([]ExpenseForecast, error) {
	rows, err := conn.Query(`
		SELECT id, partner_id, concept, description,
		       printf('%.2f', gross_amount), printf('%.2f', approved_amount),
		       approved_on, planned_date, year, subtype_code, scope, added_on, enabled
		FROM expense_forecast ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpenseForecast
	for rows.Next() {
		var f ExpenseForecast
		var approvedOn sql.NullString
		var planned, added string
		var enabled int
		if err := rows.Scan(&f.ID, &f.PartnerID, &f.Concept, &f.Description,
			&f.GrossAmount, &f.ApprovedAmount, &approvedOn, &planned, &f.Year,
			&f.SubtypeCode, &f.Scope, &added, &enabled); err != nil {
			return nil, err
		}
		if f.ApprovedOn, err = parseNullTime(approvedOn); err != nil {
			return nil, err
		}
		if f.PlannedDate, err = parseTime(planned); err != nil {
			return nil, err
		}
		if f.AddedOn, err = parseTime(added); err != nil {
			return nil, err
		}
		f.Enabled = enabled == 1
		out = append(out, f)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/adopt/legacy/ -v`
Expected: PASS (8 partners, 35 forecasts, money `1322.22` exact, etc.).

- [ ] **Step 6: Commit**

```bash
git add cmd/adopt/legacy/ testdata/.gitignore
git commit -m "feat(adopt): legacy Java-schema reader with exact money extraction"
```

---

### Task 16: Adopt tool — transform, main, validation

**Files:**
- Create: `cmd/adopt/transform/transform.go`
- Create: `cmd/adopt/main.go`
- Test: `cmd/adopt/transform/transform_test.go`
- Modify: `Makefile` (add `adopt` build target)

**Interfaces:**
- Consumes: `legacy.Read`, `db.Open`, all repositories, `model.*`.
- Produces: `transform.Run(ctx, legacyPath, destPath string) (transform.Counts, error)` — builds the new DB (goose), loads the transformed data in one transaction, writes a final MIGRATION audit event, and returns per-table counts; `cmd/adopt/main.go` wiring `--from/--to/--force`.

- [ ] **Step 1: Write the failing transform test**

Create `cmd/adopt/transform/transform_test.go`:
```go
package transform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/cmd/adopt/transform"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

const fixture = "../../../testdata/legacy-espigol.db"

func TestRun_AdoptsRealFixture(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping")
	}
	dest := filepath.Join(t.TempDir(), "espigol.db")

	counts, err := transform.Run(context.Background(), fixture, dest)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if counts.Partners != 8 || counts.Forecasts != 35 || counts.Reports != 1 {
		t.Fatalf("counts = %+v", counts)
	}

	conn, err := db.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	q := sqlc.New(conn)
	ctx := context.Background()

	// sections seeded + memberships derived
	secs, _ := persistence.NewSectionRepository(q).List(ctx)
	if len(secs) != 2 {
		t.Errorf("sections = %d, want 2", len(secs))
	}
	mem, _ := persistence.NewSectionRepository(q).ListMembershipsByPartner(ctx, 1)
	if len(mem) != 2 { // partner 1 had olive_section=1, livestock_section=1
		t.Errorf("partner 1 memberships = %d, want 2", len(mem))
	}

	// money exact, scope mapped
	fs, _ := persistence.NewForecastRepository(conn, q).ListByYear(ctx, 2026)
	if len(fs) != 35 {
		t.Errorf("forecasts = %d, want 35", len(fs))
	}
	var sawSection, sawExactReal bool
	for _, f := range fs {
		if f.Scope().Kind() == model.ScopeSection && f.Scope().SectionCode() == "oliva" {
			sawSection = true
		}
		if f.GrossAmount().String() == "1322.22" {
			sawExactReal = true
		}
	}
	if !sawSection {
		t.Error("expected at least one oliva SECTION forecast")
	}
	if !sawExactReal {
		t.Error("expected the former-REAL 1322.22 stored exactly")
	}

	// audit: 61 carried + 1 MIGRATION
	audits, _ := persistence.NewAuditLog(q).List(ctx)
	if len(audits) != 62 {
		t.Errorf("audits = %d, want 62 (61 + MIGRATION)", len(audits))
	}
}

func TestRun_RefusesExistingDest(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping")
	}
	dest := filepath.Join(t.TempDir(), "espigol.db")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Run(context.Background(), fixture, dest); err == nil {
		t.Error("expected Run to refuse an existing destination")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/adopt/transform/ -v`
Expected: FAIL — undefined `transform.Run`.

- [ ] **Step 3: Write the transform**

Create `cmd/adopt/transform/transform.go`. Structure:
- `type Counts struct { Partners, Sections, Memberships, Types, Subtypes, Windows, Forecasts, Reports, Audits int }`.
- `Run(ctx, legacyPath, destPath string) (Counts, error)`:
  1. If `destPath` exists → error (`os.Stat`). 
  2. `d, err := legacy.Read(legacyPath)`.
  3. `conn, err := db.Open(destPath)` (creates + migrates).
  4. `tx, _ := conn.BeginTx(ctx, nil)`; `defer tx.Rollback()`; `q := sqlc.New(conn).WithTx(tx)`; build repositories over `q` (repositories take `*sqlc.Queries`; the forecast repo here can insert with explicit ids — see note).
  5. Seed sections `oliva`/`ramaderia` (order 1/2).
  6. Insert windows (limits → `model.MoneyFromString`).
  7. Insert types then subtypes.
  8. Insert partners; for each, if `OliveSection` add membership `oliva`, if `LivestockSection` add `ramaderia`.
  9. Insert forecasts: map `Scope` Catalan → `model.NewScope`/`NewCommonScope`/`NewPartnerScope`/`NewSectionScope`; build `model.ExpenseForecast` with the **existing CPYYnnn id** (do not re-allocate — use a direct insert query, not `ForecastRepository.Create`). Add an `InsertForecast` path that takes the id verbatim (the existing `InsertForecast` sqlc query already takes an explicit id — call it through a mapper, bypassing `Create`'s allocation).
  10. Insert reports (carry id? `report.id` is autoincrement; inserting via `InsertReport` returns a fresh id — acceptable since nothing references report ids across the cutover; the test only checks count + content).
  11. Insert all audit events via `AuditLog.Append` (their ids are autoincrement; fine).
  12. Append one `model.AuditEvent` kind `MIGRATION`, actor `system@espigol`, entityType `Database`, entityID `adopt`, payload a JSON of counts.
  13. Count rows; if any per-table count mismatches the source dump length → return error (rolls back).
  14. `tx.Commit()`.

Catalan scope mapping helper:
```go
func mapScope(catalan string) (model.ExpenseScope, error) {
	switch catalan {
	case "Comú":
		return model.NewCommonScope(), nil
	case "Soci":
		return model.NewPartnerScope(), nil
	case "Secció d'oliva":
		return model.NewSectionScope("oliva")
	case "Secció de ramaderia":
		return model.NewSectionScope("ramaderia")
	default:
		return model.ExpenseScope{}, fmt.Errorf("unknown legacy scope %q", catalan)
	}
}
```

For inserting forecasts with their existing ids, add to `internal/adapters/persistence` a method `func (r *ForecastRepository) InsertWithID(ctx context.Context, f model.ExpenseForecast) error` that calls `r.q.InsertForecast(ctx, mapper.ForecastToInsert(f))` (no allocation), and use it from the transform. Add a one-line test for it in `forecast_repository_test.go` if not already covered (insert a forecast with a fixed id, read it back).

- [ ] **Step 4: Write main**

Create `cmd/adopt/main.go`:
```go
// Command adopt transforms the legacy espigol-java SQLite database into the new
// espigol schema. One-off cutover tool; not part of the espigol binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pjover/espigol/cmd/adopt/transform"
)

func main() {
	from := flag.String("from", "", "path to the legacy espigol-java SQLite DB")
	to := flag.String("to", "", "destination path for the new espigol.db")
	force := flag.Bool("force", false, "overwrite the destination if it exists")
	flag.Parse()

	if *from == "" || *to == "" {
		log.Fatal("adopt: --from and --to are required")
	}
	if *force {
		_ = os.Remove(*to)
	}
	counts, err := transform.Run(context.Background(), *from, *to)
	if err != nil {
		log.Fatalf("adopt: %v", err)
	}
	fmt.Printf("adopted: %+v\n", counts)
}
```

- [ ] **Step 5: Add Makefile target**

Add to `Makefile` (`.PHONY` + target, TAB-indented):
```makefile
adopt:
	go build -o bin/adopt ./cmd/adopt
```

- [ ] **Step 6: Run the transform test + full suite**

Run: `go test ./... && go build ./...`
Expected: all PASS (the adopt test asserts 8 partners, 35 forecasts, 2 sections, exact `1322.22`, oliva SECTION scope, 62 audits).

- [ ] **Step 7: Commit**

```bash
git add cmd/adopt/ internal/adapters/persistence/ Makefile
git commit -m "feat(adopt): transform legacy DB into new schema, with main and validation"
```

---

## Self-Review

**Spec coverage (against the Phase 2 design):**
- §1 decisions (Money TEXT, goose, sqlc-from-migrations, adopt tool, CPYYnnn overflow) → Tasks 1, 3, 7, 15–16.
- §2 domain model (all value types + entities + forecastid) → Tasks 1–5 (+ enums/scope in 2, forecastid in 3).
- §3 persistence (schema, sqlc, mappers, repositories, ports, next-id) → Tasks 6–14.
- §3.1 board_authorization uniqueness via expression index → Task 7 migration + Task 14 query.
- §4 adopt tool (steps, single-transaction, validation, not idempotent, `--from/--to/--force`) → Tasks 15–16.
- §5 testing (domain unit, persistence integration round-trips incl. Money exactness & next-id, adopt against frozen fixture) → tests in every task; adopt fixture in Task 15.
- §6 scope (no allocation/close/report/UI) → respected; repositories expose methods only.
- §7 file structure → matches Tasks 6–16 paths.

**Placeholder scan:** No "TBD"/"implement later". The two intentional in-test reminders (the `model.NewPartnerStub()` note in Task 9 and the `seedWindow2026` stub in Task 10) are explicitly flagged with instructions to replace/remove them before committing — they are teaching markers, not silent placeholders. Task 15 step 4 specifies the exact SQL approach (`printf('%.2f', col)`) rather than leaving money extraction vague.

**Type consistency:** Repository constructors and method signatures match the `ports` interfaces defined in Task 6; the compile-time assertions in Task 14 (`ports_check.go`) enforce this. `mapper` helper names (`FormatTimestamp`/`ParseTimestamp`/`FormatDate`/`ParseDate`/`FormatNullableTimestamp`/`ParseNullableTimestamp`) are defined in Task 8 and reused in Tasks 11–14. `forecastid.Format/ParseSeq` (Task 3) are used in Task 12. `model` constructors/accessors are consistent across model tasks and mappers.

**Note on sqlc field names:** sqlc derives Go identifiers from column names (e.g. `snapshot_json` → `SnapshotJson`, `gross_amount` → `GrossAmount`) and parameter struct names from query names (e.g. `UpsertPartnerParams`). After each `make sqlc-generate`, the implementer must read the generated `internal/adapters/persistence/sqlc` package and reconcile any mapper field reference that differs from what this plan assumed. This is called out in Tasks 8, 13, and 14.

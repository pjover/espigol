# Espígol (Go) — Phase 6: Server (socis web) — Design

**Status:** Approved for implementation · **Date:** 2026-06-29

Phase 6 of the Espígol Go rewrite. The socis-facing web server: Google OAuth (with a local
dev-login bypass), SQLite-backed sessions, soci/board forecast CRUD, and a read-only HTML
report view — plus the first real production wiring. Parent:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§2 auth/roles, §8).

**Socis-only.** All admin (windows, taxonomy, partners, sections, board-authorization
management, report generation/export) stays in the TUI (Phase 7) and is **not** in the
server. The server only *reads* the published `Report` for the HTML view.

Authoritative reference: espigol-java `adapters/web` + `adapters/auth` (Spring MVC +
Thymeleaf + OAuth2), adapted to Go `net/http` + `html/template`.

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice |
|---|---|
| Sessions | **Hand-rolled SQLite session store** (a `session` table + opaque-token cookie). |
| Auth mode | **Auto-detect**: dev-login form when `OAuth.ClientID/Secret` are absent; Google OAuth when present (dev route not registered in prod). The cookie `Secure` flag follows the same detection. |
| HTML report | **`HTMLRenderer`** in the report adapter reusing the shared Phase-5 `buildLayout`; embedded by the web handler. |
| UI | `html/template`, server-rendered, **plain forms** (POST → redirect-after-post); no JS framework / htmx. |
| Forecast CRUD | A new `application.ForecastService` (over `TxManager`) owns all soci/board CRUD + authorization. |

---

## 2. Architecture

```
internal/
├── application/
│   ├── forecast_service.go     # NEW: soci/board forecast CRUD + authorization
│   └── errors.go               # extend with ErrNoOpenWindow, ErrWindowNotOpen, ErrForbidden
├── adapters/
│   ├── auth/                   # NEW: SessionStore, OAuth2 (Google), dev-login, middleware
│   ├── report/htmlrenderer.go  # NEW: HTMLRenderer over buildLayout
│   └── web/                    # handlers + html/template pages + embedded CSS (replaces the stub)
│       └── templates/          # go:embed
└── wire/                       # NEW: production assembly for --server
```

- **`internal/wire`** — `Server(cfg) (*web.Server, error)`: `db.Open` → sqlc + repos →
  `TxManager` → `ForecastService` → report `HTMLRenderer` → auth → `web.NewServer(deps)`.
- **`web`** — `NewServer(deps) *Server` + `(*Server) Run(ctx) error`; depends on small
  interfaces, not `database/sql`.
- **`cmd/espigol --server`** calls `wire.Server(cfg)` then `srv.Run(ctx)` (graceful shutdown
  preserved). The Phase-1 stub `web.Run(ctx, cfg)` is replaced.
- **Config additions** (`internal/config`): `OAuth.RedirectURL` (Google callback URL, prod
  only). Auth mode and the cookie `Secure` flag are *derived* from whether
  `OAuth.ClientID/Secret` are set — no new flags.
- **Scope guard:** the server never imports `WindowService` or any admin operation; it only
  reads published reports.

---

## 3. Sessions & authentication (`internal/adapters/auth`)

### 3.1 Session store (hand-rolled, SQLite)

- New goose migration `db/migrations/00002_session.sql`:
  `session(token TEXT PRIMARY KEY, partner_id INTEGER NOT NULL, email TEXT NOT NULL,
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL, FOREIGN KEY(partner_id) →
  partner(id))`.
- `SessionStore` (sqlc-backed): `Create(ctx, partnerID, email) (token string, err error)` —
  32 random bytes base64url-encoded; TTL 30 days; insert. `Get(ctx, token) (Session, bool,
  error)` — returns the session only if not expired. `Delete(ctx, token) error`. Expired
  rows are treated as absent (and may be deleted opportunistically).
- Cookie `espigol_session` = the token: `HttpOnly`, `SameSite=Lax`, `Secure` (= prod mode),
  `Path=/`, `Max-Age` = TTL.

### 3.2 Flows (each ends by creating a session + cookie, then redirect to `/`)

- **Production — Google OAuth2** (`golang.org/x/oauth2` + `google`):
  - `GET /login` → random `state` in a short-lived `oauth_state` cookie; redirect to Google
    (scopes `openid email`).
  - `GET /oauth2/callback` → verify `state`; exchange `code`; fetch userinfo email;
    `PartnerRepository.FindByEmail` → found ⇒ create session; not found ⇒ **access-denied**
    page (Catalan: "aquest correu no està registrat com a soci; contacta amb el Consell
    Rector").
- **Dev (auto, when OAuth creds absent):**
  - `GET /login` → email form. `POST /dev-login` → `FindByEmail` → session or access-denied.
    `/dev-login` is **only registered in dev mode**.
- `POST /logout` → delete session row + clear cookie → redirect `/`.

### 3.3 Middleware & role

- `RequireAuth`: cookie → `SessionStore.Get` → `PartnerRepository.FindByID` → attach
  `Partner` to request `context`; miss/expired ⇒ redirect `/login`. Public routes bypass:
  `/login`, `/oauth2/callback`, `/dev-login`, `/health`, `/css/*`, `/access-denied`.
- **Role** = `Partner.BoardMember()`. No roles table; the `ForecastService` consumes the
  `Partner` (+ its `BoardAuthorization`s) to decide editability.

---

## 4. `application.ForecastService` (soci/board CRUD + authorization)

Over `TxManager`; every method runs in one `WithinTx`. The web layer does authn
(session→`Partner`) and passes the acting partner.

**OPEN year:** at most one `OPEN` window; the service finds it (`Windows.List`). All soci
writes target that year; none open ⇒ `ErrNoOpenWindow`.

**Authorization (enforced in the service, given the acting `Partner`):**
- Window must be `OPEN`, else `ErrWindowNotOpen` (Catalan "El termini ja ha finalitzat,
  contacta amb el Consell Rector").
- **Regular soci:** create/edit/delete only `PARTNER`-scope forecasts owned by themselves
  (`partnerID == actor.ID`); touching another's ⇒ `ErrForbidden`.
- **Board member:** the above, plus `COMMON`/`SECTION` forecasts only for scopes in their
  `BoardAuthorization` rows (`COMMON`, or `SECTION`+`sectionCode`); other scopes ⇒
  `ErrForbidden`.
- Subtype/section validity for the year is enforced (subtype must exist in the year's
  taxonomy; `SECTION` `sectionCode` must be an active section) — backed by DB FKs, surfaced
  as a typed error.

**Methods:**
- `Dashboard(ctx, actor) (DashboardView, error)` — OPEN year + deadline; the actor's own
  `PARTNER` forecasts; if board, the `COMMON`/`SECTION` forecasts for their authorized
  scopes; links to past published reports (closed years).
- `Create(ctx, actor, ForecastInput) (model.ExpenseForecast, error)` — validate OPEN + scope
  auth; build the scope; allocate the `CPYYnnn` id via `ForecastRepository.Create`.
  `ForecastInput{Concept, Description, GrossAmount, PlannedDate, SubtypeCode, ScopeKind,
  SectionCode}`.
- `Update(ctx, actor, id string, ForecastInput) error` — load, authorize, edit while OPEN.
- `Delete(ctx, actor, id string) error` — load, authorize, delete while OPEN.
- `Get(ctx, actor, id string) (model.ExpenseForecast, error)` — for the edit form;
  authorize read.

Each mutation writes an `AuditEvent` (`FORECAST_CREATED/EDITED/DELETED`, actor = the soci's
email). Typed errors live in `application/errors.go`.

---

## 5. Web layer (`internal/adapters/web`)

### 5.1 Routes (Go 1.22+ method routing — the full external surface)

| Route | Auth | Purpose |
|---|---|---|
| `GET /health` | public | liveness |
| `GET /login` | public | Google redirect (prod) or dev-login form (dev) |
| `GET /oauth2/callback` | public | OAuth exchange → session (prod) |
| `POST /dev-login` | public (dev only) | form login → session |
| `POST /logout` | auth | clear session |
| `GET /access-denied` | public | "not a registered soci" page |
| `GET /` | auth | dashboard (`ForecastService.Dashboard`) |
| `GET /forecasts/new` | auth | new-forecast form |
| `POST /forecasts` | auth | create |
| `GET /forecasts/{id}/edit` | auth | edit form |
| `POST /forecasts/{id}` | auth | update |
| `POST /forecasts/{id}/delete` | auth | delete |
| `GET /reports/{year}` | auth | read-only HTML report (closed years) |
| `GET /css/*` | public | embedded static CSS |

### 5.2 Handlers, templates, HTML report

- Handlers are thin: parse form/path value, read `Partner` from context, call the service.
  Typed errors map to outcomes: `ErrForbidden`→403; `ErrWindowNotOpen`→HTTP 409 Catalan
  notice; validation errors→re-render the form with the message. POSTs redirect-after-post.
- `/reports/{year}`: `ReportRepository.FindLatestByYear` → `SnapshotFromJSON` →
  `report.HTMLRenderer.Render` → embed in the page (read-only; any logged-in soci, closed
  years; everyone sees all allocations).
- Templates (`go:embed`, `html/template`, Catalan, auto-escaped): base layout, `login`,
  `dashboard`, `forecast_form` (scope selector shown only to board members, limited to their
  authorized scopes; subtype dropdown from the OPEN year's taxonomy), `report`,
  `access_denied`, `error`. Embedded CSS under `/css/`.
- **CSRF:** state-changing routes are POST-only with a `SameSite=Lax` session cookie, plus a
  per-session CSRF token rendered as a hidden form field and verified on POST.

### 5.3 HTMLRenderer (`internal/adapters/report/htmlrenderer.go`)

`HTMLRenderer.Render(rd report.ReportData) []byte` consumes the same `buildLayout` blocks as
the PDF/MD renderers (`SectionTitle`→`<h2>`, `Table`→`<table>` with the title as `<h3>`,
bold/red rows styled, `PageBreak` ignored), so the on-screen report matches the PDF/MD by
construction. Adapter-pure (domain + stdlib only).

---

## 6. Wiring, config & testing

### 6.1 Wiring

`wire.Server(cfg)` assembles the server (db.Open runs migrations incl. `00002_session.sql`).
`cmd/espigol --server` → `wire.Server(cfg)` → `srv.Run(ctx)`. `web.Server`/middleware depend
on small interfaces (`forecastService`, `reportReader` = `FindLatestByYear`, `partnerLookup`
= `FindByEmail`/`FindByID`, `SessionStore`) for testability.

### 6.2 Testing (TDD)

- **SessionStore** (temp SQLite): create→get→delete; expired token ⇒ not found.
- **ForecastService** (real SQLite via TxManager + seeded OPEN year/taxonomy/sections/
  partners/board-auths): soci creates/edits/deletes own; soci editing another's ⇒
  `ErrForbidden`; write with no OPEN window ⇒ `ErrWindowNotOpen`/`ErrNoOpenWindow`; board
  edits an authorized `COMMON`/`SECTION`, unauthorized scope ⇒ `ErrForbidden`; audit events
  written.
- **Web handlers** (`httptest`, **dev-login path**, no Google): unauthenticated ⇒ redirect
  `/login`; dev-login known email ⇒ session cookie + dashboard; unknown email ⇒
  access-denied; create/edit/delete round-trip via forms; missing/invalid CSRF ⇒ rejected;
  `GET /reports/{year}` for a closed year renders expected EU-formatted numbers; logout
  clears the session.
- The OAuth token-exchange isn't tested against real Google; it sits behind a small
  interface so the email-resolution (found/not-found) branch is tested with a fake.

### 6.3 Scope

**In:** auth (sessions + OAuth/dev), `ForecastService`, web handlers + templates + CSS, the
HTML report view + `HTMLRenderer`, production wiring, the `00002_session.sql` migration,
config `OAuth.RedirectURL`.

**Out (later):** admin/TUI (Phase 7); auto-close scheduler + email notifications (deferred);
PDF/MD file-export triggering (TUI). The server only reads published reports.

---

## 7. References

- Overview: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§2, §8).
- Phase 4: `application` (`TxManager`, `SnapshotFromJSON`), `ports`.
- Phase 5: `report` (`buildLayout`, renderers).
- Java reference: espigol-java `adapters/web`, `adapters/auth`.

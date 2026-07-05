# Espígol — TUI Redesign — Design

**Status:** Approved for implementation · **Date:** 2026-07-05

Structural redesign of the Bubble Tea admin TUI from its current plain-text layout to an
IDE Three-Panel layout with framed panels, a persistent context sidebar, true-color semantic
palette, and multi-line form field support. No changes to the application layer, domain, or
Panel interface.

---

## 1. Decisions

| Decision | Choice |
|---|---|
| Layout paradigm | IDE Three-Panel (sidebar + center panel + footer) |
| Responsive strategy | Proportional split; sidebar fixed width (≈22 chars + borders), center expands |
| Borders | Framed panels (lipgloss borders) around sidebar and center panel |
| Sidebar role | Always-visible context card (business name, year, state) + numbered panel navigator |
| Center panel | List (top ~60%) + `──` separator + inline detail (bottom ~40%) |
| Bottom area | Single-line footer — no dedicated bottom panel |
| Interaction model | Direct keybinding, L0 universal (arrows, Enter, Esc, q) + per-panel action keys |
| Panel switch | `[1-6]` global hotkeys (work from anywhere, replace Tab as panel switcher) |
| Color tier | True color (24-bit), semantic slots |
| Multi-line input | `bubbles/textarea` for fields with `Multiline: true`; `Alt+Enter` inserts newline, `Enter` advances field |
| Modal rendering | Centered overlay via `lipgloss.Place()` (replaces appended-below approach) |
| Panel interface | **Unchanged** — `Title()`, `Update()`, `View()`, `Detail()`, `Actions()` |
| Panel implementations | **Unchanged** — no edits to any `panel_*.go` file except `panel_forecasts.go` (description field) |

---

## 2. Layout

### 2.1 Structure

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ┌─ Espígol SCCL ────────┐ ┌─ Anys ─────────────────────────────────┐  │
│ │ Any: 2026             │ │  Any    Estat     Obert       Tancat   │  │
│ │ ● OBERT               │ │ ────────────────────────────────────── │  │
│ │                       │ │ ▶ 2026  OBERT     2026-01-01  -        │  │
│ │ ─────────────────     │ │   2025  TANCAT    2025-01-01  2025-... │  │
│ │ [1] Anys              │ │   2024  TANCAT    2024-01-01  2024-... │  │
│ │ [2] Socis             │ │                                        │  │
│ │ [3] Seccions          │ │ ──────────────────────────────────     │  │
│ │ [4] Tipus             │ │ Any 2026 · OBERT                       │  │
│ │ [5] Previsions        │ │ Obert: 2026-01-01                      │  │
│ │ [6] Informes          │ │ Límit corrent: 1.200,00 €              │  │
│ └───────────────────────┘ │ Límit inversió: 300,00 €              │  │
│                            └────────────────────────────────────────┘  │
│  [↑↓] navegar  [1-6] panell  [n] nou  [o] obrir  [c] tancar  [q] surt  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Sidebar (left, fixed width)

- Fixed width: 22 content chars + 2 border chars = 24 cols total.
- Always visible; not a focus target (keyboard focus lives in the center panel).
- Contents (top to bottom):
  1. Business name (`cfg.BusinessName`) — bold, `fg.emphasis`
  2. `Any: YYYY` — muted label + bold year
  3. State badge: `● OBERT` / `● TANCAT` / `● ESBORRANY` — colored per state
  4. Horizontal separator `─────────────────`
  5. Numbered panel list: `[1] Anys` … `[6] Informes`
     - Active panel entry: bold + `accent.primary`
     - Inactive entries: `fg.muted`
- Border: `fg.muted` (always, sidebar is never "focused").

### 2.3 Center panel (right, fills remaining width)

- Framed with lipgloss border; panel `Title()` rendered as the border label.
- Border color: `accent.primary` (always focused — only panel in the app that receives key input).
- Interior split:
  - **List section** (top ~60% of interior height): `p.View(width, listHeight)`
  - **Separator**: single `─` line, full interior width, `fg.muted`
  - **Detail section** (remaining height): `p.Detail()`
- If `Detail()` returns `""`, the separator is omitted and the list uses full height.

### 2.4 Footer (single line, no border)

- `fg.muted` text, key glyphs `[x]` in `fg.emphasis`.
- Built from `m.panels[m.focused].Actions()` + global keys.
- Format: `[↑↓] navegar  [1-6] panell  [n] nou  [e] edita  [q] surt`
- Context-sensitive: shows only actions valid for the focused panel right now.

### 2.5 Minimum size gate

- Below 80×24: render a single centered line `"Terminal massa petit (mínim 80×24)"` in `status.error`.

---

## 3. Interaction Model

### 3.1 Global keys (always active)

| Key | Action |
|-----|--------|
| `1`–`6` | Switch to panel N |
| `↑` / `↓` | Move selection within active panel list |
| `Enter` | Confirm / select |
| `Esc` | Close modal / cancel |
| `q` | Quit |

`Tab` is **removed** as a panel-switch key. It is only used inside modals to cycle fields.

### 3.2 Per-panel action keys

| Panel | Keys |
|-------|------|
| Anys | `n` nou any · `o` obrir · `c` tancar · `a` esmenar · `r` informe |
| Socis | `n` nou · `e` edita · `b` junta · `m` seccions |
| Seccions | `n` nou · `e` edita · `d` elimina |
| Tipus | `n` nou · `e` edita · `d` elimina *(hidden when year ≠ ESBORRANY)* |
| Previsions | `n` nou · `e` edita · `d` elimina |
| Informes | `r` generar/exportar |

### 3.3 Modal keys

| Key | Context | Action |
|-----|---------|--------|
| `Enter` | Any field | Move to next field; submit when on the last field |
| `Alt+Enter` | Textarea field | Insert newline |
| `Tab` / `Shift+Tab` | Any field | Cycle fields forward / backward |
| `y` | Confirm modal | Confirm |
| `n` / `Esc` | Confirm modal | Cancel |
| `Esc` | Form modal | Cancel without saving |

Footer inside modal: `[Enter] camp seguent  [Alt+Enter] nova línia  [Esc] cancel·la`

---

## 4. Color & Visual System

### 4.1 Semantic palette (true color)

```go
// styles.go — all color references go through these vars
var (
    colorFgDefault  = lipgloss.Color("#c0caf5") // body text
    colorFgMuted    = lipgloss.Color("#565f89") // secondary, hints, inactive
    colorFgEmphasis = lipgloss.Color("#e0e0e0") // headers, focused items

    colorBgBase      = lipgloss.Color("#1a1b26") // terminal bg (AltScreen)
    colorBgSurface   = lipgloss.Color("#24283b") // panel interior bg
    colorBgOverlay   = lipgloss.Color("#414868") // modal bg, dimmed backdrop
    colorBgSelection = lipgloss.Color("#364a82") // selected row

    colorAccent = lipgloss.Color("#7aa2f7") // focused border, active panel entry

    colorError   = lipgloss.Color("#f7768e") // errors, destructive
    colorWarning = lipgloss.Color("#e0af68") // ESBORRANY badge
    colorSuccess = lipgloss.Color("#9ece6a") // OBERT badge
    colorInfo    = lipgloss.Color("#7dcfff") // TANCAT badge

    colorDraftBg = lipgloss.Color("#3d3200") // ESBORRANY badge background
)
```

### 4.2 State badges

| State | Style |
|-------|-------|
| `ESBORRANY` | `colorWarning` fg on `colorDraftBg` bg |
| `OBERT` | `colorSuccess` bold |
| `TANCAT` | `colorInfo` bold |

Color is always paired with the text label (never color alone).

### 4.3 Border rules

| Element | Border color |
|---------|-------------|
| Sidebar | `colorFgMuted` (never focused) |
| Center panel | `colorAccent` (always the active input target) |
| Modal | `colorAccent`, rounded border |

### 4.4 Selected row

`colorBgSelection` background + `colorFgEmphasis` text. Reverse-video (`lipgloss.NewStyle().Reverse(true)`) as fallback when true color unavailable.

---

## 5. Modal Redesign

### 5.1 Centered overlay (`app.go`)

```go
// In View(), after composing the main layout string:
if m.modal != nil {
    overlay := m.modal.View()
    view = lipgloss.Place(m.width, m.height,
        lipgloss.Center, lipgloss.Center, overlay)
}
```

The backdrop is the full rendered main view at natural opacity. The modal's rounded border provides visual separation.

### 5.2 Multi-line field support (`form.go`)

`formFieldDef` gains `Multiline bool`:

```go
type formFieldDef struct {
    Label       string
    Placeholder string
    Value       string
    Multiline   bool  // if true, uses textarea.Model instead of textinput.Model
}
```

The `formField` struct holds both models; only one is active (`multiline` selects which):

```go
type formField struct {
    label     string
    multiline bool
    single    textinput.Model // used when !multiline
    multi     textarea.Model  // used when multiline
}
```

Key routing in `formModal.Update()`:

- `enter` → `blurCurrent()`, advance `m.focused`; if already on the last field, collect values and call `onSubmit`
- `alt+enter` → if the focused field is a textarea, insert `\n`
- `tab` / `shift+tab` → cycle fields

---

## 6. Files Changed

| File | Nature of change |
|------|-----------------|
| `internal/adapters/tui/styles.go` | Replace 256-color vars with true-color semantic vars (§4.1) |
| `internal/adapters/tui/app.go` | New `renderSidebar()`, redesign `View()`, `[1-6]` key handling in `Update()`, centered modal overlay |
| `internal/adapters/tui/form.go` | `Multiline bool` on `formFieldDef`, `textarea.Model` support, revised key routing |
| `internal/adapters/tui/panel_forecasts.go` | Add `Multiline: true` to the description `formFieldDef` |

**Unchanged:** `confirm.go`, `panel.go`, `panel_years.go`, `panel_partners.go`, `panel_sections.go`, `panel_taxonomy.go`, `panel_reports.go`, `deps.go`, all application services, domain, wire.

---

## 7. Out of Scope

- Backup / restore admin actions (separate future phase)
- Light terminal theme variant
- Mouse support
- Viewport scrolling for long lists (current data volumes don't warrant it)
- Animation / transitions

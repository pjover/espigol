# TUI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the Bubble Tea admin TUI from a plain-text layout to an IDE Three-Panel layout with framed panels, a persistent context sidebar, true-color semantic palette, [1-6] panel switching, centered modal overlays, and multi-line textarea support for the forecast description field.

**Architecture:** The root model (`app.go`) gains a framed sidebar (context card + panel navigator) and a framed center panel (list + separator + detail). Styles are promoted to a true-color semantic palette in `styles.go`. The generic `formModal` gains `Multiline bool` support; the bespoke `forecastFormModal` swaps its `description` field from `textinput` to `textarea`. The Panel interface is untouched; only `panel_years.go` and `panel_forecasts.go` need small changes.

**Tech Stack:** Go 1.26, bubbletea v1.3.10, lipgloss v1.1.0, bubbles v1.0.0 (`bubbles/textinput`, `bubbles/textarea`, `bubbles/help` removed from root model)

## Global Constraints

- Module: `github.com/pjover/espigol`; CGO-free; no build tags
- All user-facing strings in Catalan
- Run tests: `go test ./internal/adapters/tui/...`
- All terminal rendering via lipgloss — no raw escape sequences
- Minimum supported terminal: 80×24; below that show `"Terminal massa petit (mínim 80×24)"`
- True-color only (admin-only TUI, runs locally or over SSH with `COLORTERM=truecolor`)
- Panel interface (`Title`, `Update`, `View`, `Detail`, `Actions`) is unchanged
- `confirm.go`, `panel_partners.go`, `panel_sections.go`, `panel_taxonomy.go`, `panel_reports.go`, `deps.go`, application services, domain, wire — untouched

---

## File Map

| File | Change |
|------|--------|
| `internal/adapters/tui/styles.go` | Full rewrite — true-color semantic vars + border styles + `stateBadge()` |
| `internal/adapters/tui/app.go` | Add `yearState` field, `renderSidebar()`, redesign `View()`, `[1-6]` switching, centered modal, remove `showHelp`/`help` |
| `internal/adapters/tui/confirm.go` | Use new `modalStyle` instead of raw `lipgloss.NewStyle()` |
| `internal/adapters/tui/form.go` | `Multiline bool` on `formFieldDef`, union `formField` struct, `alt+enter` newline, `enter` = advance/submit-last |
| `internal/adapters/tui/panel_years.go` | Add `State model.WindowState` to `yearSelectedMsg`; update 3 call sites |
| `internal/adapters/tui/panel_forecasts.go` | Swap `description` from `textinput.Model` to `textarea.Model`; update key routing |
| `internal/adapters/tui/app_test.go` | Remove Tab-navigation tests; add `[1]`/`[2]` switch tests; update footer-content test |
| `internal/adapters/tui/panel_basic_test.go` | Update `submitForm` helper for new `formField` struct and enter-on-last-field behavior |
| `internal/adapters/tui/panel_admin_test.go` | Update two forecast tests to set `focused = int(fieldPlannedDate)` before pressing enter |

---

### Task 1: True-color semantic palette

**Files:**
- Rewrite: `internal/adapters/tui/styles.go`
- Modify: `internal/adapters/tui/confirm.go`

**Interfaces:**
- Produces: `sidebarStyle`, `centerStyle`, `modalStyle`, `stateBadge(model.WindowState) string` — consumed by Task 2
- Produces: renamed/updated vars `colorError`, `colorAccent`, `focusedPanelStyle`, `helpStyle`, `redStyle`, `titleStyle`, `dimStyle` — consumed by all panels (they already reference these names, so renaming internally is safe)

- [ ] **Step 1: Rewrite `styles.go`**

Replace the entire file with:

```go
package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/pjover/espigol/internal/domain/model"
)

var (
	colorFgMuted    = lipgloss.Color("#565f89")
	colorFgEmphasis = lipgloss.Color("#e0e0e0")

	colorBgSelection = lipgloss.Color("#364a82")

	colorAccent = lipgloss.Color("#7aa2f7")

	colorError   = lipgloss.Color("#f7768e")
	colorWarning = lipgloss.Color("#e0af68")
	colorSuccess = lipgloss.Color("#9ece6a")
	colorInfo    = lipgloss.Color("#7dcfff")

	colorDraftBg = lipgloss.Color("#3d3200")
)

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorFgEmphasis)
	dimStyle          = lipgloss.NewStyle().Foreground(colorFgMuted)
	redStyle          = lipgloss.NewStyle().Foreground(colorError)
	focusedPanelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpStyle         = lipgloss.NewStyle().Foreground(colorFgMuted)

	// sidebarInnerWidth is the content width inside the sidebar border+padding.
	// Outer rendered width = sidebarInnerWidth + 2 (Padding(0,1)) + 2 (border) = sidebarInnerWidth + 4.
	sidebarInnerWidth = 20

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFgMuted).
			Padding(0, 1).
			Width(sidebarInnerWidth)

	centerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
)

func stateStyle(state model.WindowState) lipgloss.Style {
	switch state {
	case model.WindowDraft:
		return lipgloss.NewStyle().Foreground(colorWarning).Background(colorDraftBg)
	case model.WindowOpen:
		return lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	case model.WindowClosed:
		return lipgloss.NewStyle().Foreground(colorInfo).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}

func stateBadge(state model.WindowState) string {
	switch state {
	case model.WindowDraft:
		return stateStyle(state).Render("● ESBORRANY")
	case model.WindowOpen:
		return stateStyle(state).Render("● OBERT")
	case model.WindowClosed:
		return stateStyle(state).Render("● TANCAT")
	default:
		return dimStyle.Render("● -")
	}
}
```

- [ ] **Step 2: Update `confirm.go` to use `modalStyle`**

In `confirm.go`, replace the raw box style in `View()`:

```go
// Before
func (m confirmModal) View() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Foreground(colorWhite)
	return box.Render(fmt.Sprintf("%s\n\n[y] sí    [n] no", m.message))
}

// After
func (m confirmModal) View() string {
	return modalStyle.Render(fmt.Sprintf("%s\n\n[y] sí    [n] no", m.message))
}
```

Remove the now-unused `"github.com/charmbracelet/lipgloss"` import from `confirm.go` only if it becomes unused after this change (it likely is, since `modalStyle` is defined in `styles.go` and lipgloss is only imported for the raw style). Keep `"fmt"` and `tea "github.com/charmbracelet/bubbletea"`.

- [ ] **Step 3: Run tests to verify compile + pass**

```bash
go test ./internal/adapters/tui/... -count=1
```

Expected: all existing tests pass (styles are purely visual; no logic changed).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/tui/styles.go internal/adapters/tui/confirm.go
git commit -m "feat(tui): true-color semantic palette and modal border style"
```

---

### Task 2: IDE layout — sidebar, [1-6] switching, centered modal

**Files:**
- Modify: `internal/adapters/tui/app.go`
- Modify: `internal/adapters/tui/panel_years.go`

**Interfaces:**
- Consumes: `sidebarStyle`, `centerStyle`, `modalStyle`, `stateBadge()` from Task 1
- Modifies: `yearSelectedMsg` — adds `State model.WindowState` field (panel_years.go sends it, rootModel reads it)

- [ ] **Step 1: Add `State` to `yearSelectedMsg` and `yearState` to `rootModel` in `app.go`**

```go
// yearSelectedMsg — add State field
type yearSelectedMsg struct {
	Year  int
	State model.WindowState
}

// yearSelectedCmd — add state parameter
func yearSelectedCmd(year int, state model.WindowState) tea.Cmd {
	return func() tea.Msg { return yearSelectedMsg{Year: year, State: state} }
}

// rootModel — add yearState, remove showHelp and help
type rootModel struct {
	deps      Deps
	panels    []Panel
	focused   int
	year      int
	yearState model.WindowState

	modal tea.Model

	width  int
	height int
}

// newRootModel — remove help.New(), showHelp init
func newRootModel(deps Deps, panels []Panel) rootModel {
	return rootModel{
		deps:    deps,
		panels:  panels,
		year:    time.Now().Year(),
	}
}
```

Remove the `"github.com/charmbracelet/bubbles/help"` import from `app.go`.

- [ ] **Step 2: Update `Update()` in `app.go`**

Replace the key-handling block. Remove `?`/showHelp toggle, remove `tab`/`shift+tab`/`left`/`right` panel switching. Add `[1-6]`. Add `yearState` update in `yearSelectedMsg` case:

```go
func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case openModalMsg:
		m.modal = msg.modal
		var cmd tea.Cmd
		if init, ok := m.modal.(interface{ Init() tea.Cmd }); ok {
			cmd = init.Init()
		}
		return m, cmd

	case modalClosedMsg:
		m.modal = nil
		return m, nil

	case yearSelectedMsg:
		m.year = msg.Year
		m.yearState = msg.State
		cmds := make([]tea.Cmd, 0, len(m.panels))
		for i, p := range m.panels {
			updated, cmd := p.Update(msg)
			m.panels[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}

	if m.modal != nil {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6":
			idx := int(keyMsg.String()[0] - '1')
			if idx >= 0 && idx < len(m.panels) {
				m.focused = idx
			}
			return m, nil
		}
	}

	if len(m.panels) == 0 {
		return m, nil
	}

	if _, isKey := msg.(tea.KeyMsg); isKey {
		panel, cmd := m.panels[m.focused].Update(msg)
		m.panels[m.focused] = panel
		return m, cmd
	}
	var cmds []tea.Cmd
	for i, p := range m.panels {
		updated, cmd := p.Update(msg)
		m.panels[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}
```

- [ ] **Step 3: Rewrite `View()` and add helpers in `app.go`**

Replace `View()`, `renderPanelList()`, `renderMain()`, `renderHelpLine()` with the new layout methods:

```go
func (m rootModel) View() string {
	if m.width > 0 && m.height > 0 && (m.width < 80 || m.height < 24) {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			redStyle.Render("Terminal massa petit (mínim 80×24)"))
	}

	sidebar := m.renderSidebar()
	center := m.renderCenter()
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", center)
	view := body + "\n" + m.renderFooter()

	if m.modal != nil {
		view = lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center, m.modal.View())
	}
	return view
}

// sidebarOuterWidth is the total rendered width of the sidebar including border+padding.
// sidebarInnerWidth (20) + 2 (Padding(0,1)) + 2 (RoundedBorder left+right) = 24.
const sidebarOuterWidth = 24

func (m rootModel) renderSidebar() string {
	var b strings.Builder

	businessName := ""
	if m.deps.Cfg != nil {
		businessName = m.deps.Cfg.BusinessName
	}
	b.WriteString(titleStyle.Render(businessName))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Any: ") + titleStyle.Render(strconv.Itoa(m.year)))
	b.WriteString("\n")
	b.WriteString(stateBadge(m.yearState))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", sidebarInnerWidth)))
	b.WriteString("\n")

	for i, p := range m.panels {
		entry := fmt.Sprintf("[%d] %s", i+1, p.Title())
		if i == m.focused {
			entry = focusedPanelStyle.Render(entry)
		} else {
			entry = dimStyle.Render(entry)
		}
		b.WriteString(entry + "\n")
	}

	return sidebarStyle.Render(b.String())
}

func (m rootModel) renderCenter() string {
	// centerInnerW = total width - sidebar outer - gap(1) - center border+padding(4)
	centerInnerW := m.width - sidebarOuterWidth - 1 - 4
	if centerInnerW < 10 {
		centerInnerW = 10
	}
	centerInnerH := m.height - 1 - 2 // reserve footer(1) + center top+bottom border(2)
	if centerInnerH < 3 {
		centerInnerH = 3
	}
	listH := centerInnerH * 60 / 100

	if len(m.panels) == 0 {
		return centerStyle.Width(centerInnerW).Render(dimStyle.Render("(cap panell)"))
	}

	p := m.panels[m.focused]
	list := p.View(centerInnerW, listH)
	detail := p.Detail()

	var content string
	if detail == "" {
		content = list
	} else {
		sep := dimStyle.Render(strings.Repeat("─", centerInnerW))
		content = list + "\n" + sep + "\n" + detail
	}

	return centerStyle.Width(centerInnerW).Render(content)
}

func (m rootModel) renderFooter() string {
	var parts []string
	if len(m.panels) > 0 {
		for _, a := range m.panels[m.focused].Actions() {
			parts = append(parts, "["+a.Key+"] "+a.Label)
		}
	}
	parts = append(parts, "[↑↓] navegar", "[1-6] panell", "[q] surt")
	return helpStyle.Render(strings.Join(parts, "  "))
}
```

Add `"fmt"` to the imports if not already present. Remove `"github.com/charmbracelet/bubbles/help"`. Keep `"strings"`, `"strconv"`, `"time"`, `lipgloss`, `tea`.

- [ ] **Step 4: Update `panel_years.go` — add `State` to `yearSelectedCmd` call sites**

In `panel_years.go`, update `yearSelectedCmd` calls (3 places):

```go
// In yearsLoadedMsg handler — after updating p.windows:
if w, ok := p.selectedWindow(); ok {
    return p, yearSelectedCmd(w.Year(), w.State())
}

// In handleKey "up":
if w, ok := p.selectedWindow(); ok {
    return p, yearSelectedCmd(w.Year(), w.State())
}

// In handleKey "down":
if w, ok := p.selectedWindow(); ok {
    return p, yearSelectedCmd(w.Year(), w.State())
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/adapters/tui/... -count=1
```

Expected: compile errors in `app_test.go` (Tab-navigation tests reference removed behaviour). This is expected — Task 3 fixes those tests.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/tui/app.go internal/adapters/tui/panel_years.go
git commit -m "feat(tui): IDE three-panel layout with sidebar, [1-6] switching, centered modals"
```

---

### Task 3: Update navigation tests in `app_test.go`

**Files:**
- Modify: `internal/adapters/tui/app_test.go`

**Interfaces:**
- Consumes: new `rootModel` API from Task 2 (`[1-6]` switching, no `showHelp`)

- [ ] **Step 1: Remove Tab-navigation and help-toggle tests**

Delete these four test functions entirely from `app_test.go`:
- `TestRootModel_TabAdvancesFocus`
- `TestRootModel_TabWrapsAround`
- `TestRootModel_ShiftTabMovesBackward`
- `TestRootModel_QuestionMarkTogglesHelp`

- [ ] **Step 2: Add `[1-6]` panel-switch test**

Add after `TestRootModel_StartsFocusedOnFirstPanel`:

```go
func TestRootModel_NumberKeySwitchesPanel(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())

	// stubPanels has 3 panels: Anys(0), Socis(1), Seccions(2).
	// Press "2" → focus Socis.
	updated, _ := m.Update(keyMsg("2"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Socis" {
		t.Errorf("after '2', FocusedTitle() = %q, want %q", got, "Socis")
	}

	// Press "1" → back to Anys.
	updated, _ = m.Update(keyMsg("1"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Anys" {
		t.Errorf("after '1', FocusedTitle() = %q, want %q", got, "Anys")
	}

	// Press "6" → out of range (only 3 panels); focus stays on Anys.
	updated, _ = m.Update(keyMsg("6"))
	m = updated.(rootModel)
	if got := m.FocusedTitle(); got != "Anys" {
		t.Errorf("after out-of-range '6', FocusedTitle() = %q, want %q (unchanged)", got, "Anys")
	}
}
```

- [ ] **Step 3: Update `TestRootModel_ActionsAppearInHelpLine`**

The footer format changed from `n: nou` to `[n] nou`:

```go
func TestRootModel_ActionsAppearInHelpLine(t *testing.T) {
	m := newRootModel(Deps{}, stubPanels())
	if !strings.Contains(m.View(), "[n] nou") {
		t.Errorf("View() = %q, want it to contain the focused panel's action in [key] label format", m.View())
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/adapters/tui/... -count=1
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/tui/app_test.go
git commit -m "test(tui): update navigation tests for [1-6] panel switching"
```

---

### Task 4: Generic form multiline support

**Files:**
- Modify: `internal/adapters/tui/form.go`
- Modify: `internal/adapters/tui/panel_basic_test.go`

**Interfaces:**
- Produces: `formFieldDef.Multiline bool` — consumed by Task 5 (panel_forecasts.go uses the generic formModal only indirectly; the bespoke modal is separate)
- Produces: `formField.value() string` helper — consumed by updated `submitForm` test helper
- `submitForm(t, form, values)` signature is unchanged; internal implementation changes

- [ ] **Step 1: Rewrite `form.go`**

Replace the entire file:

```go
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formFieldDef describes a field to create via newFormModal.
type formFieldDef struct {
	Label       string
	Placeholder string
	Value       string
	Multiline   bool // if true, renders as textarea; Alt+Enter inserts newline
}

// formField holds one labelled input — either single-line (textinput) or
// multi-line (textarea). Exactly one of single/multi is active, selected by
// the multiline flag.
type formField struct {
	label     string
	multiline bool
	single    textinput.Model
	multi     textarea.Model
}

func (f formField) value() string {
	if f.multiline {
		return f.multi.Value()
	}
	return f.single.Value()
}

// formModal is a generic ordered list of labelled input fields.
type formModal struct {
	title    string
	fields   []formField
	focused  int
	onSubmit func(values map[string]string) tea.Cmd
}

func newFormModal(title string, fieldDefs []formFieldDef, onSubmit func(values map[string]string) tea.Cmd) formModal {
	fields := make([]formField, len(fieldDefs))
	for i, def := range fieldDefs {
		if def.Multiline {
			ta := textarea.New()
			ta.Placeholder = def.Placeholder
			ta.SetValue(def.Value)
			ta.ShowLineNumbers = false
			ta.CharLimit = 0
			if i == 0 {
				ta.Focus()
			}
			fields[i] = formField{label: def.Label, multiline: true, multi: ta}
		} else {
			ti := textinput.New()
			ti.Placeholder = def.Placeholder
			ti.SetValue(def.Value)
			if i == 0 {
				ti.Focus()
			}
			fields[i] = formField{label: def.Label, single: ti}
		}
	}
	return formModal{title: title, fields: fields, onSubmit: onSubmit}
}

// Init implements tea.Model.
func (m formModal) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m formModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		if len(m.fields) > 0 {
			cmd = m.updateActive(msg)
		}
		return m, cmd
	}

	switch keyMsg.String() {
	case "esc":
		return m, closeModalCmd
	case "enter":
		if len(m.fields) == 0 {
			return m, closeModalCmd
		}
		if m.focused == len(m.fields)-1 {
			return m, tea.Batch(m.collectAndSubmit(), closeModalCmd)
		}
		m.blurCurrent()
		m.focused = (m.focused + 1) % len(m.fields)
		m.focusCurrent()
		return m, nil
	case "alt+enter":
		if len(m.fields) > 0 && m.fields[m.focused].multiline {
			var cmd tea.Cmd
			m.fields[m.focused].multi, cmd = m.fields[m.focused].multi.Update(
				tea.KeyMsg{Type: tea.KeyEnter})
			return m, cmd
		}
		return m, nil
	case "tab", "down":
		m.blurCurrent()
		m.focused = (m.focused + 1) % max(len(m.fields), 1)
		m.focusCurrent()
		return m, nil
	case "shift+tab", "up":
		m.blurCurrent()
		m.focused = (m.focused - 1 + len(m.fields)) % max(len(m.fields), 1)
		m.focusCurrent()
		return m, nil
	}

	var cmd tea.Cmd
	if len(m.fields) > 0 {
		cmd = m.updateActive(msg)
	}
	return m, cmd
}

func (m *formModal) collectAndSubmit() tea.Cmd {
	values := make(map[string]string, len(m.fields))
	for _, f := range m.fields {
		values[f.label] = f.value()
	}
	if m.onSubmit != nil {
		return m.onSubmit(values)
	}
	return nil
}

func (m *formModal) updateActive(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f := &m.fields[m.focused]
	if f.multiline {
		f.multi, cmd = f.multi.Update(msg)
	} else {
		f.single, cmd = f.single.Update(msg)
	}
	return cmd
}

func (m *formModal) blurCurrent() {
	if m.focused < 0 || m.focused >= len(m.fields) {
		return
	}
	f := &m.fields[m.focused]
	if f.multiline {
		f.multi.Blur()
	} else {
		f.single.Blur()
	}
}

func (m *formModal) focusCurrent() {
	if m.focused < 0 || m.focused >= len(m.fields) {
		return
	}
	f := &m.fields[m.focused]
	if f.multiline {
		f.multi.Focus()
	} else {
		f.single.Focus()
	}
}

// View implements tea.Model.
func (m formModal) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")
	for i, f := range m.fields {
		label := f.label
		if i == m.focused {
			label = focusedPanelStyle.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		b.WriteString(label + ": ")
		if f.multiline {
			b.WriteString(f.multi.View())
		} else {
			b.WriteString(f.single.View())
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[Enter] camp seguent  [Alt+Enter] nova línia  [Esc] cancel·la"))
	return modalStyle.Render(b.String())
}
```

- [ ] **Step 2: Update `submitForm` helper in `panel_basic_test.go`**

Replace `submitForm`:

```go
// submitForm sets each field's value then submits from the last field.
func submitForm(t *testing.T, form formModal, values map[string]string) tea.Msg {
	t.Helper()
	for i, f := range form.fields {
		val, ok := values[f.label]
		if !ok {
			continue
		}
		if f.multiline {
			form.fields[i].multi.SetValue(val)
		} else {
			form.fields[i].single.SetValue(val)
		}
	}
	// Enter on the last field submits.
	form.focused = len(form.fields) - 1
	var model tea.Model = form
	_, cmd := model.Update(pKey("enter"))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from submitting the form")
	}
	return cmd()
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/adapters/tui/... -count=1
```

Expected: all tests pass. The generic formModal tests (partners, sections, taxonomy via `submitForm`) exercise the new enter-on-last-field submit path.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/tui/form.go internal/adapters/tui/panel_basic_test.go
git commit -m "feat(tui): generic form multiline textarea support with Alt+Enter newline"
```

---

### Task 5: Forecast description — swap to textarea

**Files:**
- Modify: `internal/adapters/tui/panel_forecasts.go`
- Modify: `internal/adapters/tui/panel_admin_test.go`

**Interfaces:**
- Consumes: `bubbles/textarea` (already pulled in by Task 4 via go.mod/bubbles v1.0.0)
- No Panel interface changes

- [ ] **Step 1: Swap `description` from `textinput.Model` to `textarea.Model` in `forecastFormModal`**

In `panel_forecasts.go`:

1. Add `"github.com/charmbracelet/bubbles/textarea"` to imports.

2. In `forecastFormModal` struct, change the `description` field type:

```go
// Before
description textinput.Model

// After
description textarea.Model
```

3. In `newForecastFormModal`, replace the `description` initialization:

```go
// Before
description.Placeholder = "Descripció"
// ... (description was textinput.New())

// After
description := textarea.New()
description.Placeholder = "Descripció"
description.ShowLineNumbers = false
description.CharLimit = 0
if existing != nil {
    description.SetValue(existing.Description())
}
```

Remove `description` from the block that creates `textinput.New()` instances for `concept`, `gross`, `plannedDate`.

4. Update `Init()`:

```go
func (m *forecastFormModal) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.description.Init())
}
```

5. Update `Update()` — change `enter` to advance-on-non-last / submit-on-last, add `alt+enter`:

```go
case "enter":
	// Selector fields and last text field submit; all others advance focus.
	if m.focused == int(forecastFormFieldCount)-1 {
		if cmd := m.submit(); cmd != nil {
			return m, tea.Batch(cmd, closeModalCmd)
		}
		return m, nil
	}
	m.blurCurrent()
	m.focused = (m.focused + 1) % int(forecastFormFieldCount)
	m.focusCurrent()
	return m, nil
case "alt+enter":
	if forecastFormField(m.focused) == fieldDescription {
		var cmd tea.Cmd
		m.description, cmd = m.description.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m, cmd
	}
	return m, nil
```

6. Update `updateTextField()` for `fieldDescription`:

```go
case fieldDescription:
	var cmd tea.Cmd
	m.description, cmd = m.description.Update(msg)
	return cmd
```

7. Update `blurCurrent()` for `fieldDescription`:

```go
case fieldDescription:
	m.description.Blur()
```

8. Update `focusCurrent()` for `fieldDescription`:

```go
case fieldDescription:
	m.description.Focus()
```

9. Update `textLine` call in `View()` — textarea has its own `View()`:

```go
// Before
b.WriteString(m.textLine("Descripció", fieldDescription, m.description))

// After — textarea renders differently; use a label + View() inline
descLabel := dimStyle.Render("Descripció")
if forecastFormField(m.focused) == fieldDescription {
    descLabel = focusedPanelStyle.Render("Descripció")
}
b.WriteString(fmt.Sprintf("%s:\n%s\n", descLabel, m.description.View()))
```

10. Update the modal box in `View()` to use `modalStyle`:

```go
// Before
box := lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    Padding(1, 2)
return box.Render(b.String())

// After
return modalStyle.Render(b.String())
```

Remove the now-unused `"github.com/charmbracelet/lipgloss"` import from `panel_forecasts.go` if `modalStyle` covers all usages (check — `lipgloss` is also used in `textLine` for the styled label via `dimStyle`/`focusedPanelStyle`; but those are defined in `styles.go` and don't need a direct lipgloss import in this file). Remove `lipgloss` import if it's no longer referenced directly.

- [ ] **Step 2: Update `panel_admin_test.go` — two forecast form tests**

**`TestForecastsPanel_InvalidGrossKeepsFormOpenWithError`:**

```go
func TestForecastsPanel_InvalidGrossKeepsFormOpenWithError(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
	form := runCmd(t, cmd).(openModalMsg).modal.(*forecastFormModal)
	form.concept.SetValue("Adobs")
	form.gross.SetValue("not-a-number")
	form.plannedDate.SetValue("2026-04-15")
	// Move to the last field so Enter triggers submit (and validation).
	form.focused = int(fieldPlannedDate)

	var tm tea.Model = form
	updated, submitCmd := tm.Update(pKey("enter"))
	if submitCmd != nil {
		t.Fatalf("expected nil cmd (form stays open) on invalid gross, got a command")
	}
	if fm := updated.(*forecastFormModal); !strings.Contains(fm.View(), "Import brut no vàlid") {
		t.Errorf("expected an inline gross-validation error in the form view; got:\n%s", fm.View())
	}
}
```

**`TestForecastsPanel_CreateViaFormCallsAdminCreate`:**

```go
func TestForecastsPanel_CreateViaFormCallsAdminCreate(t *testing.T) {
	deps, q := testDeps(t)
	seedDraftTaxonomyYear(t, q, 2026)
	seedForecastPartner(t, deps, 1)

	p := NewForecastsPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	loaded := runCmd(t, cmd).(forecastsLoadedMsg)
	p, _ = p.Update(loaded)

	_, cmd = p.Update(pKey("n"))
	msg := runCmd(t, cmd)
	modalMsg, ok := msg.(openModalMsg)
	if !ok {
		t.Fatalf("expected openModalMsg, got %T", msg)
	}
	form, ok := modalMsg.modal.(*forecastFormModal)
	if !ok {
		t.Fatalf("expected *forecastFormModal, got %T", modalMsg.modal)
	}
	form.concept.SetValue("Adobs")
	form.description.SetValue("Adobs de primavera") // textarea.SetValue — same API
	form.gross.SetValue("250.00")
	form.plannedDate.SetValue("2026-04-15")
	// Move to the last field so Enter triggers submit.
	form.focused = int(fieldPlannedDate)

	var tm tea.Model = form
	_, submitCmd := tm.Update(pKey("enter"))
	if submitCmd == nil {
		t.Fatal("expected a non-nil cmd from submitting the forecast form")
	}
	msgs := drainMsgs(submitCmd())
	var sawCreated bool
	for _, m := range msgs {
		if fl, ok := m.(forecastsLoadedMsg); ok {
			if fl.err != nil {
				t.Fatalf("unexpected error from AdminCreate: %v", fl.err)
			}
			for _, f := range fl.forecasts {
				if f.Concept() == "Adobs" && f.PartnerID() == 1 {
					sawCreated = true
				}
			}
		}
	}
	if !sawCreated {
		t.Error("expected the newly created forecast (via AdminCreate) to appear in the reloaded list")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/adapters/tui/... -count=1
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/tui/panel_forecasts.go internal/adapters/tui/panel_admin_test.go
git commit -m "feat(tui): textarea for forecast description with Alt+Enter newline"
```

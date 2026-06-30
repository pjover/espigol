package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// formField is a single labelled input in a formModal.
type formField struct {
	label string
	input textinput.Model
}

// formModal is a generic, ordered list of labelled text fields. Panels
// (Task 11/12) build one with newFormModal to collect free-text input (a
// name, an email, an amount as a string, etc.) before calling the relevant
// application service. "Selector" fields (e.g. picking a partner or a
// subtype from a list) are out of scope for this generic modal; panels that
// need them can layer their own tea.Model and still honour the same
// modalClosedMsg/onSubmit convention.
type formModal struct {
	title    string
	fields   []formField
	focused  int
	onSubmit func(values map[string]string) tea.Cmd
}

// newFormModal builds a form modal with the given title and ordered
// (label, initial value) field pairs. onSubmit receives the field values
// keyed by label when the user presses enter.
func newFormModal(title string, fieldDefs []formFieldDef, onSubmit func(values map[string]string) tea.Cmd) formModal {
	fields := make([]formField, len(fieldDefs))
	for i, def := range fieldDefs {
		ti := textinput.New()
		ti.Placeholder = def.Placeholder
		ti.SetValue(def.Value)
		if i == 0 {
			ti.Focus()
		}
		fields[i] = formField{label: def.Label, input: ti}
	}
	return formModal{title: title, fields: fields, onSubmit: onSubmit}
}

// formFieldDef describes a field to create via newFormModal.
type formFieldDef struct {
	Label       string
	Placeholder string
	Value       string
}

// Init implements tea.Model.
func (m formModal) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m formModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		if len(m.fields) > 0 {
			m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(msg)
		}
		return m, cmd
	}

	switch keyMsg.String() {
	case "esc":
		return m, closeModalCmd
	case "enter":
		values := make(map[string]string, len(m.fields))
		for _, f := range m.fields {
			values[f.label] = f.input.Value()
		}
		var submitCmd tea.Cmd
		if m.onSubmit != nil {
			submitCmd = m.onSubmit(values)
		}
		return m, tea.Batch(submitCmd, closeModalCmd)
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
		m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(msg)
	}
	return m, cmd
}

func (m *formModal) blurCurrent() {
	if m.focused >= 0 && m.focused < len(m.fields) {
		m.fields[m.focused].input.Blur()
	}
}

func (m *formModal) focusCurrent() {
	if m.focused >= 0 && m.focused < len(m.fields) {
		m.fields[m.focused].input.Focus()
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
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(f.input.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("tab/shift+tab: mou camp · enter: desa · esc: cancel·la"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)
	return box.Render(b.String())
}

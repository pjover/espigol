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

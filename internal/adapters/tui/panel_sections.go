package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// sectionsLoadedMsg carries the result of (re)loading the sections list.
// err is either a plain load failure, or — when this message follows a
// mutation (reloadCmd) — the mutation's own error, which always takes
// priority over the reload's (almost always nil) error. This is what lets
// Detail() show why a Create/Update was rejected instead of silently
// discarding it.
type sectionsLoadedMsg struct {
	sections []model.Section
	err      error
}

// sectionsPanel is the "Seccions" panel: lists sections and lets the admin
// create/edit them.
type sectionsPanel struct {
	deps     Deps
	sections []model.Section
	selected int
	err      error
}

// NewSectionsPanel builds the Seccions (sections) panel.
func NewSectionsPanel(deps Deps) Panel {
	return sectionsPanel{deps: deps}
}

func (p sectionsPanel) Title() string { return "Seccions" }

func (p sectionsPanel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		sections, err := p.deps.Sections.List(context.Background())
		return sectionsLoadedMsg{sections: sections, err: err}
	}
}

// reloadCmd wraps a service call: if run fails, the mutation error is what
// gets surfaced to the panel (the list is still reloaded so the view stays
// fresh, but the reload's own nil error must never clobber a real failure).
func (p sectionsPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		mutateErr := run(context.Background())
		sections, loadErr := p.deps.Sections.List(context.Background())
		err := loadErr
		if mutateErr != nil {
			err = mutateErr
		}
		return sectionsLoadedMsg{sections: sections, err: err}
	}
}

func (p sectionsPanel) selectedSection() (model.Section, bool) {
	if p.selected < 0 || p.selected >= len(p.sections) {
		return model.Section{}, false
	}
	return p.sections[p.selected], true
}

func (p sectionsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()

	case sectionsLoadedMsg:
		p.sections = msg.sections
		p.err = msg.err
		if p.selected >= len(p.sections) {
			p.selected = max(0, len(p.sections)-1)
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p sectionsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.sections)-1 {
			p.selected++
		}
		return p, nil
	case "n":
		return p, openModalCmd(p.sectionForm(nil))
	case "e":
		if existing, ok := p.selectedSection(); ok {
			return p, openModalCmd(p.sectionForm(&existing))
		}
		return p, nil
	}
	return p, nil
}

// sectionForm builds the create/edit section form. existing == nil means
// "create" (the code field is editable); otherwise it's "edit" (code fixed).
func (p sectionsPanel) sectionForm(existing *model.Section) formModal {
	title := "Nova secció"
	code, label, active, order := "", "", "true", "0"
	if existing != nil {
		title = "Edita secció"
		code = existing.Code()
		label = existing.Label()
		active = strconv.FormatBool(existing.Active())
		order = strconv.Itoa(existing.DisplayOrder())
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Codi", Placeholder: "vinya", Value: code})
	}
	fields = append(fields,
		formFieldDef{Label: "Etiqueta", Placeholder: "Secció de vinya", Value: label},
		formFieldDef{Label: "Activa", Placeholder: "true/false", Value: active},
		formFieldDef{Label: "Ordre", Placeholder: "0", Value: order},
	)

	onSubmit := func(values map[string]string) tea.Cmd {
		activeVal, err := strconv.ParseBool(strings.TrimSpace(values["Activa"]))
		if err != nil {
			return nil
		}
		orderVal, err := strconv.Atoi(strings.TrimSpace(values["Ordre"]))
		if err != nil {
			return nil
		}
		input := application.SectionInput{
			Label:        values["Etiqueta"],
			Active:       activeVal,
			DisplayOrder: orderVal,
		}

		if existing == nil {
			input.Code = strings.TrimSpace(values["Codi"])
			return p.reloadCmd(func(ctx context.Context) error {
				_, err := p.deps.Sections.Create(ctx, input)
				return err
			})
		}
		input.Code = existing.Code()
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Sections.Update(ctx, existing.Code(), input)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

func (p sectionsPanel) View(width, height int) string {
	if len(p.sections) == 0 {
		return dimStyle.Render("(cap secció)")
	}
	off := scrollOffset(p.selected, len(p.sections), height)
	end := off + height
	if end > len(p.sections) {
		end = len(p.sections)
	}
	var b strings.Builder
	for i, sec := range p.sections[off:end] {
		idx := off + i
		state := "activa"
		if !sec.Active() {
			state = "inactiva"
		}
		raw := truncate(fmt.Sprintf("%s  %s  (%s)", sec.Code(), sec.Label(), state), width-2)
		var line string
		switch {
		case idx == p.selected:
			line = focusedPanelStyle.Render("> " + raw)
		case !sec.Active():
			line = "  " + dimStyle.Render(raw)
		default:
			line = "  " + raw
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (p sectionsPanel) Detail() string {
	sec, ok := p.selectedSection()
	if !ok {
		return errDetail(p.err)
	}
	active := "no"
	if sec.Active() {
		active = "sí"
	}
	detail := fmt.Sprintf("Codi %s  ·  %s  ·  Activa: %s  ·  Ordre: %d", sec.Code(), sec.Label(), active, sec.DisplayOrder())
	if errLine := errDetail(p.err); errLine != "" {
		detail += "\n" + errLine
	}
	return detail
}

func (p sectionsPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nova"},
		{Key: "e", Label: "edita"},
	}
}

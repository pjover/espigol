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

func (p sectionsPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		_ = run(context.Background())
		sections, err := p.deps.Sections.List(context.Background())
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
	var b strings.Builder
	for i, sec := range p.sections {
		state := "activa"
		if !sec.Active() {
			state = "inactiva"
		}
		line := fmt.Sprintf("%s  %s  (%s)", sec.Code(), sec.Label(), state)
		if !sec.Active() {
			line = dimStyle.Render(line)
		}
		if i == p.selected {
			line = focusedPanelStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (p sectionsPanel) Detail() string {
	sec, ok := p.selectedSection()
	if !ok {
		if p.err != nil {
			return redStyle.Render(p.err.Error())
		}
		return ""
	}
	active := "no"
	if sec.Active() {
		active = "sí"
	}
	return fmt.Sprintf("Codi %s  ·  %s  ·  Activa: %s  ·  Ordre: %d", sec.Code(), sec.Label(), active, sec.DisplayOrder())
}

func (p sectionsPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nova"},
		{Key: "e", Label: "edita"},
	}
}

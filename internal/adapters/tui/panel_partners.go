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

// partnersLoadedMsg carries the result of (re)loading the partner list.
type partnersLoadedMsg struct {
	partners []model.Partner
	err      error
}

// partnersPanel is the "Socis" panel: lists partners and lets the admin
// create/edit them, toggle board membership, and edit section memberships.
type partnersPanel struct {
	deps     Deps
	partners []model.Partner
	selected int
	err      error
}

// NewPartnersPanel builds the Socis (partners) panel.
func NewPartnersPanel(deps Deps) Panel {
	return partnersPanel{deps: deps}
}

func (p partnersPanel) Title() string { return "Socis" }

func (p partnersPanel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		partners, err := p.deps.Partners.List(context.Background())
		return partnersLoadedMsg{partners: partners, err: err}
	}
}

func (p partnersPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		_ = run(context.Background())
		partners, err := p.deps.Partners.List(context.Background())
		return partnersLoadedMsg{partners: partners, err: err}
	}
}

func (p partnersPanel) selectedPartner() (model.Partner, bool) {
	if p.selected < 0 || p.selected >= len(p.partners) {
		return model.Partner{}, false
	}
	return p.partners[p.selected], true
}

func (p partnersPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()

	case partnersLoadedMsg:
		p.partners = msg.partners
		p.err = msg.err
		if p.selected >= len(p.partners) {
			p.selected = max(0, len(p.partners)-1)
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p partnersPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.partners)-1 {
			p.selected++
		}
		return p, nil
	case "n":
		return p, openModalCmd(p.partnerForm(nil))
	case "e":
		if existing, ok := p.selectedPartner(); ok {
			return p, openModalCmd(p.partnerForm(&existing))
		}
		return p, nil
	case "b":
		existing, ok := p.selectedPartner()
		if !ok {
			return p, nil
		}
		return p, p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Partners.SetBoardMember(ctx, existing.ID(), !existing.BoardMember())
		})
	case "m":
		existing, ok := p.selectedPartner()
		if !ok {
			return p, nil
		}
		return p, openModalCmd(p.membershipsForm(existing))
	}
	return p, nil
}

// partnerForm builds the create/edit partner form. existing == nil means
// "create" (the id field is editable); otherwise it's "edit" (id fixed).
func (p partnersPanel) partnerForm(existing *model.Partner) formModal {
	title := "Nou soci"
	idValue, name, surname, vat, email, mobile, ptype, ria := "", "", "", "", "", "", "", ""
	if existing != nil {
		title = "Edita soci"
		idValue = strconv.Itoa(existing.ID())
		name = existing.Name()
		surname = existing.Surname()
		vat = existing.VatCode()
		email = existing.Email()
		mobile = existing.Mobile()
		ptype = string(existing.PartnerType())
		ria = strconv.Itoa(existing.RiaNumber())
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Id", Placeholder: "1", Value: idValue})
	}
	fields = append(fields,
		formFieldDef{Label: "Nom", Placeholder: "Nom", Value: name},
		formFieldDef{Label: "Cognoms", Placeholder: "Cognoms", Value: surname},
		formFieldDef{Label: "NIF", Placeholder: "12345678A", Value: vat},
		formFieldDef{Label: "Email", Placeholder: "soci@example.com", Value: email},
		formFieldDef{Label: "Mobil", Placeholder: "600000000", Value: mobile},
		formFieldDef{Label: "Tipus", Placeholder: "Productor", Value: ptype},
		formFieldDef{Label: "Num RIA", Placeholder: "0", Value: ria},
	)

	onSubmit := func(values map[string]string) tea.Cmd {
		ptVal, err := model.ParsePartnerType(strings.TrimSpace(values["Tipus"]))
		if err != nil {
			return nil
		}
		riaVal, err := strconv.Atoi(strings.TrimSpace(values["Num RIA"]))
		if err != nil {
			riaVal = 0
		}
		input := application.PartnerInput{
			Name:        values["Nom"],
			Surname:     values["Cognoms"],
			VatCode:     values["NIF"],
			Email:       values["Email"],
			Mobile:      values["Mobil"],
			PartnerType: ptVal,
			RiaNumber:   riaVal,
		}
		if existing != nil {
			// Update preserves the current board-member flag; "b" is the
			// dedicated action for toggling it, the form doesn't expose it.
			input.BoardMember = existing.BoardMember()
		}

		if existing == nil {
			id, err := strconv.Atoi(strings.TrimSpace(values["Id"]))
			if err != nil {
				return nil
			}
			input.ID = id
			return p.reloadCmd(func(ctx context.Context) error {
				_, err := p.deps.Partners.Create(ctx, input)
				return err
			})
		}
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Partners.Update(ctx, existing.ID(), input)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

// membershipsForm builds a single-field form listing the partner's current
// section memberships as a comma-separated list of codes; submitting calls
// SetSectionMemberships with the parsed codes.
func (p partnersPanel) membershipsForm(partner model.Partner) formModal {
	fields := []formFieldDef{
		{Label: "Seccions (codis separats per comes)", Placeholder: "vinya,oli", Value: ""},
	}
	onSubmit := func(values map[string]string) tea.Cmd {
		raw := values["Seccions (codis separats per comes)"]
		var codes []string
		for _, c := range strings.Split(raw, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				codes = append(codes, c)
			}
		}
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Partners.SetSectionMemberships(ctx, partner.ID(), codes)
		})
	}
	return newFormModal(fmt.Sprintf("Seccions de %s %s", partner.Name(), partner.Surname()), fields, onSubmit)
}

func (p partnersPanel) View(width, height int) string {
	if len(p.partners) == 0 {
		return dimStyle.Render("(cap soci)")
	}
	var b strings.Builder
	for i, partner := range p.partners {
		board := ""
		if partner.BoardMember() {
			board = " [junta]"
		}
		line := fmt.Sprintf("%d  %s %s%s", partner.ID(), partner.Name(), partner.Surname(), board)
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

func (p partnersPanel) Detail() string {
	partner, ok := p.selectedPartner()
	if !ok {
		if p.err != nil {
			return redStyle.Render(p.err.Error())
		}
		return ""
	}
	return fmt.Sprintf("Id %d  ·  %s %s  ·  %s  ·  %s  ·  %s  ·  Tipus: %s  ·  RIA: %d",
		partner.ID(), partner.Name(), partner.Surname(), partner.VatCode(), partner.Email(), partner.Mobile(),
		partner.PartnerType(), partner.RiaNumber())
}

func (p partnersPanel) Actions() []Action {
	return []Action{
		{Key: "n", Label: "nou"},
		{Key: "e", Label: "edita"},
		{Key: "b", Label: "junta"},
		{Key: "m", Label: "seccions"},
	}
}

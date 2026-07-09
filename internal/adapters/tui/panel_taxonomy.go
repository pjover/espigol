package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// taxonomyItem is a single row in the Tipus i subtipus list: either an
// ExpenseType or an ExpenseSubtype, flattened into one displayable/selectable
// sequence (types first, then their subtypes, matching how the admin thinks
// about the hierarchy).
type taxonomyItem struct {
	isType  bool
	typ     model.ExpenseType
	subtype model.ExpenseSubtype
}

// taxonomyLoadedMsg carries the result of (re)loading the year-context's
// types+subtypes plus its window state. err is either a plain load failure,
// or — when this message follows a mutation (reloadCmd) — the mutation's own
// error, which always takes priority over the reload's (almost always nil)
// error. Mirrors yearsLoadedMsg/sectionsLoadedMsg/partnersLoadedMsg's
// convention (see panel_years.go's doc comment).
type taxonomyLoadedMsg struct {
	year     int
	items    []taxonomyItem
	state    model.WindowState
	hasState bool
	err      error
}

// taxonomyPanel is the "Tipus i subtipus" panel: lists the year-context's
// expense types and subtypes and lets the admin create/edit/delete them.
// Mutations are only allowed while the year-context window is DRAFT — the
// TaxonomyService enforces this (ErrTaxonomyLocked) but the panel also gates
// its own Actions() so the key hints disappear outside DRAFT, with a Catalan
// notice shown instead.
//
// Selecting a type vs. a subtype: "n" always creates a new type unless a
// subtype row is currently selected (then it creates a new subtype under
// that row's type) — see handleKey's "n" case. "t" is a dedicated key to
// force-create a type regardless of selection. This keeps a single "n" key
// usable without a separate type/subtype toggle mode.
type taxonomyPanel struct {
	deps     Deps
	year     int
	items    []taxonomyItem
	selected int
	state    model.WindowState
	hasState bool
	err      error
}

// NewTaxonomyPanel builds the Tipus i subtipus panel.
func NewTaxonomyPanel(deps Deps) Panel {
	return taxonomyPanel{deps: deps}
}

func (p taxonomyPanel) Title() string { return "Taxonomia" }

// loadYear loads the year's types+subtypes (flattened, types then their
// subtypes) and its window state.
func (p taxonomyPanel) loadYear(ctx context.Context, year int) ([]taxonomyItem, model.WindowState, bool, error) {
	types, err := p.deps.Taxonomy.ListTypes(ctx, year)
	if err != nil {
		return nil, "", false, err
	}
	subtypes, err := p.deps.Taxonomy.ListSubtypes(ctx, year)
	if err != nil {
		return nil, "", false, err
	}
	var items []taxonomyItem
	for _, t := range types {
		items = append(items, taxonomyItem{isType: true, typ: t})
		for _, st := range subtypes {
			if st.TypeCode() == t.Code() {
				items = append(items, taxonomyItem{isType: false, subtype: st})
			}
		}
	}

	windows, err := p.deps.Windows.List(ctx)
	if err != nil {
		return items, "", false, err
	}
	for _, w := range windows {
		if w.Year() == year {
			return items, w.State(), true, nil
		}
	}
	return items, "", false, nil
}

func (p taxonomyPanel) loadCmd() tea.Cmd {
	year := p.year
	return func() tea.Msg {
		items, state, hasState, err := p.loadYear(context.Background(), year)
		return taxonomyLoadedMsg{year: year, items: items, state: state, hasState: hasState, err: err}
	}
}

// reloadCmd wraps a service call: if run fails, the mutation error is what
// gets surfaced to the panel (the list is still reloaded so the view stays
// fresh, but the reload's own nil error must never clobber a real failure).
// Mirrors panel_sections.go/panel_partners.go/panel_years.go's convention.
func (p taxonomyPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	year := p.year
	return func() tea.Msg {
		mutateErr := run(context.Background())
		items, state, hasState, loadErr := p.loadYear(context.Background(), year)
		err := loadErr
		if mutateErr != nil {
			err = mutateErr
		}
		return taxonomyLoadedMsg{year: year, items: items, state: state, hasState: hasState, err: err}
	}
}

func (p taxonomyPanel) selectedItem() (taxonomyItem, bool) {
	if p.selected < 0 || p.selected >= len(p.items) {
		return taxonomyItem{}, false
	}
	return p.items[p.selected], true
}

// isDraft reports whether the year-context window is known to be DRAFT.
// Mutations (and the corresponding Actions()) are only offered when true.
func (p taxonomyPanel) isDraft() bool {
	return p.hasState && p.state == model.WindowDraft
}

func (p taxonomyPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()

	case yearSelectedMsg:
		p.year = msg.Year
		p.selected = 0
		return p, p.loadCmd()

	case taxonomyLoadedMsg:
		if msg.year != p.year {
			// Stale reply from a year we've since navigated away from.
			return p, nil
		}
		p.items = msg.items
		p.state = msg.state
		p.hasState = msg.hasState
		p.err = msg.err
		if p.selected >= len(p.items) {
			p.selected = max(0, len(p.items)-1)
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p taxonomyPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < len(p.items)-1 {
			p.selected++
		}
		return p, nil
	}
	if !p.isDraft() {
		// Mutations are locked outside DRAFT; ignore n/e/d silently (Actions()
		// doesn't advertise them either, so the only path here is a stray
		// keypress).
		return p, nil
	}
	switch msg.String() {
	case "n":
		if item, ok := p.selectedItem(); ok && !item.isType {
			return p, openModalCmd(p.subtypeForm(nil, item.subtype.TypeCode()))
		}
		return p, openModalCmd(p.typeForm(nil))
	case "t":
		return p, openModalCmd(p.typeForm(nil))
	case "e":
		item, ok := p.selectedItem()
		if !ok {
			return p, nil
		}
		if item.isType {
			return p, openModalCmd(p.typeForm(&item.typ))
		}
		return p, openModalCmd(p.subtypeForm(&item.subtype, item.subtype.TypeCode()))
	case "d":
		item, ok := p.selectedItem()
		if !ok {
			return p, nil
		}
		if item.isType {
			code := item.typ.Code()
			onConfirm := p.reloadCmd(func(ctx context.Context) error {
				return p.deps.Taxonomy.DeleteType(ctx, p.year, code)
			})
			return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar el tipus %s?", code), onConfirm))
		}
		code := item.subtype.Code()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Taxonomy.DeleteSubtype(ctx, p.year, code)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar el subtipus %s?", code), onConfirm))
	}
	return p, nil
}

// typeForm builds the create/edit ExpenseType form. existing == nil means
// "create" (the code field is editable); otherwise it's "edit" (code fixed).
func (p taxonomyPanel) typeForm(existing *model.ExpenseType) formModal {
	title := "Nou tipus"
	code, label, category := "", "", string(model.CategoryCurrent)
	if existing != nil {
		title = "Edita tipus"
		code = existing.Code()
		label = existing.Label()
		category = string(existing.Category())
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Codi", Placeholder: "A", Value: code})
	}
	fields = append(fields,
		formFieldDef{Label: "Etiqueta", Placeholder: "Despeses corrents", Value: label},
		formFieldDef{Label: "Categoria", Placeholder: "CURRENT/INVESTMENT", Value: category},
	)

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		cat, err := model.ParseExpenseCategory(strings.TrimSpace(values["Categoria"]))
		if err != nil {
			return nil
		}
		in := application.TypeInput{
			Year:     year,
			Label:    values["Etiqueta"],
			Category: cat,
		}
		if existing == nil {
			in.Code = strings.TrimSpace(values["Codi"])
			return p.reloadCmd(func(ctx context.Context) error {
				return p.deps.Taxonomy.CreateType(ctx, in)
			})
		}
		in.Code = existing.Code()
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Taxonomy.UpdateType(ctx, in)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

// subtypeForm builds the create/edit ExpenseSubtype form. existing == nil
// means "create" (the code field is editable); otherwise it's "edit" (code
// fixed). typeCode prefills the parent type (editable as text since the
// generic formModal has no selector widget — see form.go's doc comment).
func (p taxonomyPanel) subtypeForm(existing *model.ExpenseSubtype, typeCode string) formModal {
	title := "Nou subtipus"
	code, label := "", ""
	if existing != nil {
		title = "Edita subtipus"
		code = existing.Code()
		label = existing.Label()
		typeCode = existing.TypeCode()
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Codi", Placeholder: "a1", Value: code})
	}
	fields = append(fields,
		formFieldDef{Label: "Etiqueta", Placeholder: "Adobs", Value: label},
		formFieldDef{Label: "Tipus", Placeholder: "A", Value: typeCode},
	)

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		in := application.SubtypeInput{
			Year:     year,
			Label:    values["Etiqueta"],
			TypeCode: strings.TrimSpace(values["Tipus"]),
		}
		if existing == nil {
			in.Code = strings.TrimSpace(values["Codi"])
			return p.reloadCmd(func(ctx context.Context) error {
				return p.deps.Taxonomy.CreateSubtype(ctx, in)
			})
		}
		in.Code = existing.Code()
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Taxonomy.UpdateSubtype(ctx, in)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

func (p taxonomyPanel) View(width, height int) string {
	if len(p.items) == 0 {
		return dimStyle.Render("(cap tipus ni subtipus)")
	}
	// Reserve 2 rows at the bottom for the non-draft note when needed.
	listH := height
	if !p.isDraft() && height > 2 {
		listH = height - 2
	}
	off := scrollOffset(p.selected, len(p.items), listH)
	end := off + listH
	if end > len(p.items) {
		end = len(p.items)
	}
	var b strings.Builder
	for i, item := range p.items[off:end] {
		idx := off + i
		var raw string
		if item.isType {
			raw = truncate(fmt.Sprintf("%s  %s  (%s)", item.typ.Code(), item.typ.Label(), item.typ.Category()), width-4)
		} else {
			raw = truncate(fmt.Sprintf("  %s  %s", item.subtype.Code(), item.subtype.Label()), width-4)
		}
		var line string
		switch {
		case idx == p.selected:
			line = focusedPanelStyle.Render("> " + raw)
		case !item.isType:
			line = "  " + dimStyle.Render(raw)
		default:
			line = "  " + raw
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if !p.isDraft() {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(truncate("Tipus i subtipus només editable en esborrany (DRAFT).", width)))
	}
	return b.String()
}

func (p taxonomyPanel) Detail() string {
	item, ok := p.selectedItem()
	if !ok {
		return errDetail(p.err)
	}
	var detail string
	if item.isType {
		detail = fmt.Sprintf("Tipus %s  ·  %s  ·  Categoria: %s", item.typ.Code(), item.typ.Label(), item.typ.Category())
	} else {
		detail = fmt.Sprintf("Subtipus %s  ·  %s  ·  Tipus pare: %s", item.subtype.Code(), item.subtype.Label(), item.subtype.TypeCode())
	}
	if errLine := errDetail(p.err); errLine != "" {
		detail += "\n" + errLine
	}
	return detail
}

func (p taxonomyPanel) Actions() []Action {
	if !p.isDraft() {
		return nil
	}
	return []Action{
		{Key: "n", Label: "nou"},
		{Key: "t", Label: "nou tipus"},
		{Key: "e", Label: "edita"},
		{Key: "d", Label: "elimina"},
	}
}

package web

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// baseData is the common data embedded in every authed page.
type baseData struct {
	BusinessName string
	CSRF         string
}

func (s *Server) baseFor(r *http.Request) baseData {
	token := sessionToken(r)
	return baseData{
		BusinessName: s.deps.Cfg.BusinessName,
		CSRF:         csrfToken(token),
	}
}

// sessionToken reads the raw session cookie value from the request.
func sessionToken(r *http.Request) string {
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// checkCSRF verifies the CSRF token on a POST. On failure it writes 403 and returns false.
func (s *Server) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	if !verifyCSRF(r, sessionToken(r)) {
		http.Error(w, "token CSRF invàlid", http.StatusForbidden)
		return false
	}
	return true
}

// handleAccessDenied renders the access-denied page.
func (s *Server) handleAccessDenied(w http.ResponseWriter, r *http.Request) {
	render(w, "access_denied", nil)
}

// handleLogout deletes the session and clears the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = s.deps.Sessions.Delete(r.Context(), c.Value)
	}
	auth.ClearSessionCookie(w, s.deps.Secure)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Dashboard ---

type dashboardData struct {
	baseData
	View application.DashboardView
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	view, err := s.deps.Forecasts.Dashboard(r.Context(), actor)
	if err != nil {
		s.renderError(w, r, err, "error carregant el tauler")
		return
	}
	render(w, "dashboard", dashboardData{
		baseData: s.baseFor(r),
		View:     view,
	})
}

// --- Forecast form helpers ---

type forecastFormData struct {
	BusinessName string
	CSRF         string
	Forecast     forecastFormFields
	Subtypes     []model.ExpenseSubtype
	IsBoard      bool
	Error        string
}

// forecastFormFields holds the display values for the form (strings, not model types).
type forecastFormFields struct {
	ID          string
	Concept     string
	Description string
	GrossAmount string
	PlannedDate string // "2006-01-02"
	SubtypeCode string
	ScopeKind   string
	SectionCode string
}

func forecastFieldsFromModel(f model.ExpenseForecast) forecastFormFields {
	scopeKind := "PARTNER"
	switch f.Scope().Kind() {
	case model.ScopeCommon:
		scopeKind = "COMMON"
	case model.ScopeSection:
		scopeKind = "SECTION"
	}
	return forecastFormFields{
		ID:          f.ID(),
		Concept:     f.Concept(),
		Description: f.Description(),
		GrossAmount: f.GrossAmount().String(),
		PlannedDate: f.PlannedDate().Format("2006-01-02"),
		SubtypeCode: f.SubtypeCode(),
		ScopeKind:   scopeKind,
		SectionCode: f.Scope().SectionCode(),
	}
}

func (s *Server) loadSubtypes(r *http.Request, year int) []model.ExpenseSubtype {
	if year == 0 {
		return nil
	}
	subs, err := s.deps.Taxonomy.ListSubtypes(r.Context(), year)
	if err != nil {
		log.Printf("web: loading subtypes for year %d: %v", year, err)
		return nil
	}
	return subs
}

// openYear attempts to get the current open year by loading the dashboard view.
// Returns 0 if there is no open year.
func (s *Server) openYear(r *http.Request, actor model.Partner) int {
	view, err := s.deps.Forecasts.Dashboard(r.Context(), actor)
	if err != nil {
		return 0
	}
	return view.Year
}

// --- New forecast ---

func (s *Server) handleForecastNew(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	year := s.openYear(r, actor)
	render(w, "forecast_form", forecastFormData{
		BusinessName: s.deps.Cfg.BusinessName,
		CSRF:         csrfToken(sessionToken(r)),
		Forecast:     forecastFormFields{ScopeKind: "PARTNER"},
		Subtypes:     s.loadSubtypes(r, year),
		IsBoard:      actor.BoardMember(),
	})
}

// --- Create forecast ---

func (s *Server) handleForecastCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	in, fields, parseErr := parseForecastInput(r, actor)
	if parseErr != "" {
		year := s.openYear(r, actor)
		render(w, "forecast_form", forecastFormData{
			BusinessName: s.deps.Cfg.BusinessName,
			CSRF:         csrfToken(sessionToken(r)),
			Forecast:     fields,
			Subtypes:     s.loadSubtypes(r, year),
			IsBoard:      actor.BoardMember(),
			Error:        parseErr,
		})
		return
	}

	_, err := s.deps.Forecasts.Create(r.Context(), actor, in)
	if err != nil {
		if isWindowError(err) {
			http.Error(w, "el termini ja ha finalitzat, contacta amb el Consell Rector", http.StatusConflict)
			return
		}
		if errors.Is(err, application.ErrForbidden) {
			http.Error(w, "no teniu permís per a aquest àmbit", http.StatusForbidden)
			return
		}
		year := s.openYear(r, actor)
		render(w, "forecast_form", forecastFormData{
			BusinessName: s.deps.Cfg.BusinessName,
			CSRF:         csrfToken(sessionToken(r)),
			Forecast:     fields,
			Subtypes:     s.loadSubtypes(r, year),
			IsBoard:      actor.BoardMember(),
			Error:        err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Edit forecast ---

func (s *Server) handleForecastEdit(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	f, err := s.deps.Forecasts.Get(r.Context(), actor, id)
	if err != nil {
		if errors.Is(err, application.ErrForecastNotFound) {
			http.Error(w, "previsió no trobada", http.StatusNotFound)
			return
		}
		if errors.Is(err, application.ErrForbidden) {
			http.Error(w, "no teniu permís", http.StatusForbidden)
			return
		}
		s.renderError(w, r, err, "error carregant la previsió")
		return
	}
	year := s.openYear(r, actor)
	render(w, "forecast_form", forecastFormData{
		BusinessName: s.deps.Cfg.BusinessName,
		CSRF:         csrfToken(sessionToken(r)),
		Forecast:     forecastFieldsFromModel(f),
		Subtypes:     s.loadSubtypes(r, year),
		IsBoard:      actor.BoardMember(),
	})
}

// --- Update forecast ---

func (s *Server) handleForecastUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")

	in, fields, parseErr := parseForecastInput(r, actor)
	fields.ID = id
	if parseErr != "" {
		year := s.openYear(r, actor)
		render(w, "forecast_form", forecastFormData{
			BusinessName: s.deps.Cfg.BusinessName,
			CSRF:         csrfToken(sessionToken(r)),
			Forecast:     fields,
			Subtypes:     s.loadSubtypes(r, year),
			IsBoard:      actor.BoardMember(),
			Error:        parseErr,
		})
		return
	}

	err := s.deps.Forecasts.Update(r.Context(), actor, id, in)
	if err != nil {
		if isWindowError(err) {
			http.Error(w, "el termini ja ha finalitzat, contacta amb el Consell Rector", http.StatusConflict)
			return
		}
		if errors.Is(err, application.ErrForbidden) {
			http.Error(w, "no teniu permís per a aquest àmbit", http.StatusForbidden)
			return
		}
		if errors.Is(err, application.ErrForecastNotFound) {
			http.Error(w, "previsió no trobada", http.StatusNotFound)
			return
		}
		year := s.openYear(r, actor)
		render(w, "forecast_form", forecastFormData{
			BusinessName: s.deps.Cfg.BusinessName,
			CSRF:         csrfToken(sessionToken(r)),
			Forecast:     fields,
			Subtypes:     s.loadSubtypes(r, year),
			IsBoard:      actor.BoardMember(),
			Error:        err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Delete forecast ---

func (s *Server) handleForecastDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	actor, ok := auth.PartnerFrom(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	id := r.PathValue("id")
	err := s.deps.Forecasts.Delete(r.Context(), actor, id)
	if err != nil {
		if isWindowError(err) {
			http.Error(w, "el termini ja ha finalitzat, contacta amb el Consell Rector", http.StatusConflict)
			return
		}
		if errors.Is(err, application.ErrForbidden) {
			http.Error(w, "no teniu permís per a aquest àmbit", http.StatusForbidden)
			return
		}
		if errors.Is(err, application.ErrForecastNotFound) {
			http.Error(w, "previsió no trobada", http.StatusNotFound)
			return
		}
		s.renderError(w, r, err, "error eliminant la previsió")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Report ---

type reportData struct {
	BusinessName string
	Year         int
	ReportHTML   template.HTML
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	yearStr := r.PathValue("year")
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		http.Error(w, "any invàlid", http.StatusBadRequest)
		return
	}

	rep, ok, err := s.deps.Reports.FindLatestByYear(r.Context(), year)
	if err != nil {
		s.renderError(w, r, err, "error carregant l'informe")
		return
	}
	if !ok {
		http.Error(w, "informe no trobat per a l'any indicat", http.StatusNotFound)
		return
	}

	rd, err := application.SnapshotFromJSON(rep.SnapshotJSON())
	if err != nil {
		s.renderError(w, r, err, "error deserialitzant l'informe")
		return
	}

	htmlBytes := s.deps.HTML.Render(rd)
	render(w, "report", reportData{
		BusinessName: s.deps.Cfg.BusinessName,
		Year:         year,
		ReportHTML:   template.HTML(htmlBytes), //nolint:gosec // HTML from our own renderer
	})
}

// --- Helpers ---

// isWindowError returns true for both ErrNoOpenWindow and ErrWindowNotOpen.
func isWindowError(err error) bool {
	return errors.Is(err, application.ErrNoOpenWindow) || errors.Is(err, application.ErrWindowNotOpen)
}

// parseForecastInput parses the HTTP form into a ForecastInput.
// Returns the input, display fields (for re-rendering), and an error message (empty on success).
func parseForecastInput(r *http.Request, actor model.Partner) (application.ForecastInput, forecastFormFields, string) {
	if err := r.ParseForm(); err != nil {
		return application.ForecastInput{}, forecastFormFields{}, "error llegint el formulari"
	}

	concept := r.FormValue("concept")
	description := r.FormValue("description")
	grossStr := r.FormValue("gross_amount")
	dateStr := r.FormValue("planned_date")
	subtypeCode := r.FormValue("subtype_code")
	scopeKindStr := r.FormValue("scope_kind")
	sectionCode := r.FormValue("section_code")

	fields := forecastFormFields{
		Concept:     concept,
		Description: description,
		GrossAmount: grossStr,
		PlannedDate: dateStr,
		SubtypeCode: subtypeCode,
		ScopeKind:   scopeKindStr,
		SectionCode: sectionCode,
	}

	if concept == "" {
		return application.ForecastInput{}, fields, "el concepte és obligatori"
	}
	if subtypeCode == "" {
		return application.ForecastInput{}, fields, "el subtipus és obligatori"
	}

	gross, err := model.MoneyFromString(grossStr)
	if err != nil {
		return application.ForecastInput{}, fields, "import brut invàlid: " + err.Error()
	}

	plannedDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return application.ForecastInput{}, fields, "data prevista invàlida (format AAAA-MM-DD)"
	}

	// Determine scope
	var scopeKind model.ScopeKind
	if actor.BoardMember() {
		switch scopeKindStr {
		case "COMMON":
			scopeKind = model.ScopeCommon
		case "SECTION":
			scopeKind = model.ScopeSection
		default:
			scopeKind = model.ScopePartner
		}
	} else {
		scopeKind = model.ScopePartner
	}

	in := application.ForecastInput{
		Concept:     concept,
		Description: description,
		GrossAmount: gross,
		PlannedDate: plannedDate,
		SubtypeCode: subtypeCode,
		ScopeKind:   scopeKind,
		SectionCode: sectionCode,
	}
	return in, fields, ""
}

// renderError renders the error page for unexpected errors.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, err error, context string) {
	log.Printf("web: %s: %v", context, err)
	type errorPageData struct {
		Message string
	}
	renderStatus(w, http.StatusInternalServerError, "error", errorPageData{Message: "Error intern del servidor. Torneu-ho a intentar."})
}

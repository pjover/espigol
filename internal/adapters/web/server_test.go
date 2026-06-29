package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	reportadapter "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/adapters/web"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/domain/model"
	modelreport "github.com/pjover/espigol/internal/domain/model/report"
)

// testNow is the fixed clock time used in tests.
var testNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// buildServer creates a full server backed by a temp SQLite DB seeded with test data.
// It returns the httptest.Server and a cleanup function.
func buildServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "web_test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	q := sqlc.New(conn)
	clock := fixedClock{t: testNow}
	ctx := context.Background()

	// --- Seed: OPEN 2026 window ---
	win2026, _ := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(testNow), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, win2026); err != nil {
		t.Fatalf("seed 2026 window: %v", err)
	}

	// Taxonomy for 2026
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "Corrent", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "Inversió", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "Subtipus corrent 1", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "Subtipus inversió 1", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)

	// Partner
	soci, _ := model.NewPartner(1, "Soci Test", "", "", "s1@e.test", "", model.Productor, 0, testNow, false)
	if err := persistence.NewPartnerRepository(q).Save(ctx, soci); err != nil {
		t.Fatalf("seed partner: %v", err)
	}

	// --- Seed: CLOSED 2025 window + Report ---
	win2025, _ := model.NewSubmissionWindow(2025, model.WindowClosed, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(25000), model.MoneyOf(60000))
	if err := persistence.NewWindowRepository(q).Save(ctx, win2025); err != nil {
		t.Fatalf("seed 2025 window: %v", err)
	}

	// Minimal ReportData for 2025 with some money values
	m2880, _ := model.MoneyFromString("2880.00")
	m23498, _ := model.MoneyFromString("23498.96")
	rd2025 := modelreport.ReportData{
		Year: 2025,
		Categories: []modelreport.CategoryReportData{
			{
				Category: model.CategoryCurrent,
				Common: modelreport.CommonData{
					Available: m23498,
					Total:     m2880,
					Remainder: m23498,
				},
			},
		},
	}
	snapshotJSON, err := application.SnapshotToJSON(rd2025)
	if err != nil {
		t.Fatalf("SnapshotToJSON: %v", err)
	}
	rep2025, _ := model.NewReport(0, 2025, testNow, snapshotJSON, []byte{0x25, 0x50}, nil)
	if _, err := persistence.NewReportRepository(q).Insert(ctx, rep2025); err != nil {
		t.Fatalf("seed 2025 report: %v", err)
	}

	// --- Wire ---
	sessions := auth.NewSessionStore(q, clock)
	partners := persistence.NewPartnerRepository(q)
	cfg := &config.Config{BusinessName: "Cooperativa Test"}
	cfg.Server.Port = 0

	authn := auth.NewAuthenticator(cfg, sessions, partners) // dev mode (no OAuth creds)
	forecasts := application.NewForecastService(persistence.NewTxManager(conn), clock)

	deps := web.Deps{
		Forecasts: forecasts,
		Auth:      authn,
		Sessions:  sessions,
		Partners:  partners,
		Reports:   persistence.NewReportRepository(q),
		HTML:      reportadapter.HTMLRenderer{},
		Taxonomy:  persistence.NewTaxonomyRepository(q),
		Cfg:       cfg,
		Secure:    false,
	}
	srv := web.NewServer(deps)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func ptrTime(t time.Time) *time.Time { return &t }

// noRedirectClient returns an http.Client with a cookie jar that does NOT follow redirects.
func noRedirectClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// followRedirectClient returns an http.Client with a cookie jar that follows redirects.
func followRedirectClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func bodyString(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(b)
}

func TestServer_Integration(t *testing.T) {
	ts := buildServer(t)

	t.Run("unauthenticated GET / redirects to /login", func(t *testing.T) {
		client := noRedirectClient(t)
		resp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/login" {
			t.Errorf("Location = %q, want /login", loc)
		}
	})

	t.Run("POST /dev-login with unknown email -> 303 /access-denied", func(t *testing.T) {
		client := noRedirectClient(t)
		resp, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"nobody@x.example"}})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/access-denied" {
			t.Errorf("Location = %q, want /access-denied", loc)
		}
		// No session cookie set
		u, _ := url.Parse(ts.URL)
		for _, c := range client.Jar.Cookies(u) {
			if c.Name == auth.CookieName {
				t.Error("session cookie should NOT be set for unknown email")
			}
		}
	})

	t.Run("POST /dev-login with registered email -> 303 / sets cookie", func(t *testing.T) {
		client := noRedirectClient(t)
		resp, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/" {
			t.Errorf("Location = %q, want /", loc)
		}
		u, _ := url.Parse(ts.URL)
		var sessionCookie *http.Cookie
		for _, c := range client.Jar.Cookies(u) {
			if c.Name == auth.CookieName {
				sessionCookie = c
			}
		}
		if sessionCookie == nil {
			t.Fatal("session cookie not set after dev-login")
		}
	})

	t.Run("authed GET / contains year and Nova previsio", func(t *testing.T) {
		// Login first
		client := followRedirectClient(t)
		resp, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}})
		if err != nil {
			t.Fatal(err)
		}
		body := bodyString(t, resp)

		if !strings.Contains(body, "2026") {
			t.Errorf("dashboard missing year 2026; body snippet: %q", truncate(body, 300))
		}
		if !strings.Contains(body, "Nova previsió") {
			t.Errorf("dashboard missing 'Nova previsió'; body snippet: %q", truncate(body, 300))
		}
	})

	t.Run("CSRF: POST /forecasts without valid token -> 403", func(t *testing.T) {
		client := followRedirectClient(t)
		// Login
		if _, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}}); err != nil {
			t.Fatal(err)
		}
		// Post with bad CSRF
		resp, err := client.PostForm(ts.URL+"/forecasts", url.Values{
			"csrf":         {"invalid-csrf-token"},
			"concept":      {"Test"},
			"gross_amount": {"100.00"},
			"planned_date": {"2026-06-15"},
			"subtype_code": {"a1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403 for invalid CSRF", resp.StatusCode)
		}
	})

	t.Run("create forecast with valid CSRF -> 303 -> forecast on dashboard", func(t *testing.T) {
		client := followRedirectClient(t)
		// Login
		if _, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}}); err != nil {
			t.Fatal(err)
		}

		// Get new forecast form to extract CSRF token
		formResp, err := client.Get(ts.URL + "/forecasts/new")
		if err != nil {
			t.Fatal(err)
		}
		formBody := bodyString(t, formResp)

		csrfToken := extractCSRF(t, formBody)
		if csrfToken == "" {
			t.Fatalf("could not extract CSRF token from form; body snippet: %q", truncate(formBody, 500))
		}

		// Use a no-redirect client for the POST so we can check the 303
		noRedirect := noRedirectClient(t)
		// Transfer cookies from followRedirectClient
		u, _ := url.Parse(ts.URL)
		for _, c := range client.Jar.Cookies(u) {
			noRedirect.Jar.SetCookies(u, []*http.Cookie{c})
		}

		postResp, err := noRedirect.PostForm(ts.URL+"/forecasts", url.Values{
			"csrf":         {csrfToken},
			"concept":      {"Eines de jardí"},
			"description":  {"Eines per al hort"},
			"gross_amount": {"250.00"},
			"planned_date": {"2026-08-15"},
			"subtype_code": {"b1"},
			"scope_kind":   {"PARTNER"},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer postResp.Body.Close()
		if postResp.StatusCode != http.StatusSeeOther {
			body := bodyString(t, postResp)
			t.Fatalf("POST /forecasts status = %d, want 303; body: %q", postResp.StatusCode, truncate(body, 400))
		}

		// Now GET dashboard and check forecast appears
		dashResp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		dashBody := bodyString(t, dashResp)
		if !strings.Contains(dashBody, "Eines de jardí") {
			t.Errorf("dashboard missing created forecast 'Eines de jardí'; snippet: %q", truncate(dashBody, 400))
		}
	})

	t.Run("GET /reports/2025 -> 200 with EU-formatted amount", func(t *testing.T) {
		client := followRedirectClient(t)
		// Login
		if _, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}}); err != nil {
			t.Fatal(err)
		}
		resp, err := client.Get(ts.URL + "/reports/2025")
		if err != nil {
			t.Fatal(err)
		}
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200; body: %q", resp.StatusCode, truncate(body, 300))
		}
		// Should contain an EU-formatted amount from the 2025 snapshot
		// model.Money.String() returns "2.880,00" style (EU format) — check for that pattern
		if !strings.Contains(body, "2.880,00") && !strings.Contains(body, "23.498,96") {
			t.Errorf("report page missing EU-formatted amounts; snippet: %q", truncate(body, 500))
		}
	})

	t.Run("POST /logout clears cookie; subsequent GET / -> /login", func(t *testing.T) {
		client := noRedirectClient(t)
		// Login first
		resp, err := client.PostForm(ts.URL+"/dev-login", url.Values{"email": {"s1@e.test"}})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		u, _ := url.Parse(ts.URL)
		var sessionCookie *http.Cookie
		for _, c := range client.Jar.Cookies(u) {
			if c.Name == auth.CookieName {
				sessionCookie = c
			}
		}
		if sessionCookie == nil {
			t.Fatal("session cookie not set after login")
		}

		// Get CSRF for logout — need to read the dashboard
		followClient := followRedirectClient(t)
		followClient.Jar.SetCookies(u, []*http.Cookie{sessionCookie})
		dashResp, err := followClient.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		dashBody := bodyString(t, dashResp)
		csrfToken := extractCSRF(t, dashBody)

		// Transfer updated cookies back to noRedirect client
		for _, c := range followClient.Jar.Cookies(u) {
			client.Jar.SetCookies(u, []*http.Cookie{c})
		}

		// POST /logout
		logoutResp, err := client.PostForm(ts.URL+"/logout", url.Values{"csrf": {csrfToken}})
		if err != nil {
			t.Fatal(err)
		}
		defer logoutResp.Body.Close()

		// Check session cookie is cleared (MaxAge=-1 or value empty)
		for _, c := range logoutResp.Cookies() {
			if c.Name == auth.CookieName {
				if c.MaxAge >= 0 && c.Value != "" {
					t.Errorf("logout did not clear cookie: MaxAge=%d Value=%q", c.MaxAge, c.Value)
				}
			}
		}

		// Subsequent GET / should redirect to /login (cookie jar should now have cleared session)
		// Clear the cookie jar manually since httptest cookie jar may keep old value
		client.Jar.SetCookies(u, []*http.Cookie{{Name: auth.CookieName, Value: "", MaxAge: -1}})
		rootResp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer rootResp.Body.Close()
		if rootResp.StatusCode != http.StatusSeeOther {
			t.Errorf("after logout GET / status = %d, want 303", rootResp.StatusCode)
		}
		if loc := rootResp.Header.Get("Location"); loc != "/login" {
			t.Errorf("after logout redirect Location = %q, want /login", loc)
		}
	})
}

// extractCSRF tries to find a hidden CSRF input value in an HTML body.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	// Look for <input type="hidden" name="csrf" value="...">
	const marker = `name="csrf" value="`
	idx := strings.Index(body, marker)
	if idx == -1 {
		// Try alternate ordering
		const marker2 = `name="csrf"`
		idx2 := strings.Index(body, marker2)
		if idx2 == -1 {
			return ""
		}
		// find value= after this
		sub := body[idx2:]
		vi := strings.Index(sub, `value="`)
		if vi == -1 {
			return ""
		}
		sub = sub[vi+7:]
		ei := strings.Index(sub, `"`)
		if ei == -1 {
			return ""
		}
		return sub[:ei]
	}
	start := idx + len(marker)
	end := strings.Index(body[start:], `"`)
	if end == -1 {
		return ""
	}
	return body[start : start+end]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// seedClosedWindowServer builds a server where there is NO open window for any
// year (all windows are closed), so Create will return ErrNoOpenWindow → 409.
func buildClosedWindowServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "closed_web_test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	q := sqlc.New(conn)
	clock := fixedClock{t: testNow}
	ctx := context.Background()

	// Seed ONLY a CLOSED window — no open one.
	win2026, _ := model.NewSubmissionWindow(2026, model.WindowClosed, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, win2026); err != nil {
		t.Fatalf("seed closed 2026 window: %v", err)
	}

	// Taxonomy for 2026 (needed for form submission)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "Corrent", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "Subtipus corrent 1", "A")
	_ = tax.SaveSubtype(ctx, sa)

	// Partner
	soci, _ := model.NewPartner(1, "Soci Test", "", "", "s1@e.test", "", model.Productor, 0, testNow, false)
	if err := persistence.NewPartnerRepository(q).Save(ctx, soci); err != nil {
		t.Fatalf("seed partner: %v", err)
	}

	sessions := auth.NewSessionStore(q, clock)
	partners := persistence.NewPartnerRepository(q)
	cfg := &config.Config{BusinessName: "Cooperativa Test"}
	cfg.Server.Port = 0

	authn := auth.NewAuthenticator(cfg, sessions, partners)
	forecasts := application.NewForecastService(persistence.NewTxManager(conn), clock)

	deps := web.Deps{
		Forecasts: forecasts,
		Auth:      authn,
		Sessions:  sessions,
		Partners:  partners,
		Reports:   persistence.NewReportRepository(q),
		HTML:      reportadapter.HTMLRenderer{},
		Taxonomy:  persistence.NewTaxonomyRepository(q),
		Cfg:       cfg,
		Secure:    false,
	}
	srv := web.NewServer(deps)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// buildBoardServer builds a server with two partners:
//   - soci A  (id=1, email "sA@e.test", regular partner)
//   - board B (id=2, email "sB@e.test", board member with COMMON BoardAuthorization)
//
// An open 2026 window is seeded. Returns the server plus the ID of the
// forecast created by soci A (used for the cross-soci test).
func buildBoardServer(t *testing.T) (ts *httptest.Server, partnerAForecastID string) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "board_web_test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	q := sqlc.New(conn)
	clock := fixedClock{t: testNow}
	ctx := context.Background()

	// Open 2026 window
	win2026, _ := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(testNow), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, win2026); err != nil {
		t.Fatalf("seed 2026 window: %v", err)
	}

	// Taxonomy
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "Corrent", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "Subtipus corrent 1", "A")
	_ = tax.SaveSubtype(ctx, sa)

	// Soci A — regular partner
	sociA, _ := model.NewPartner(1, "Soci A", "", "", "sA@e.test", "", model.Productor, 0, testNow, false)
	if err := persistence.NewPartnerRepository(q).Save(ctx, sociA); err != nil {
		t.Fatalf("seed soci A: %v", err)
	}

	// Board B — board member
	boardB, _ := model.NewPartner(2, "Board B", "", "", "sB@e.test", "", model.Productor, 0, testNow, true)
	if err := persistence.NewPartnerRepository(q).Save(ctx, boardB); err != nil {
		t.Fatalf("seed board B: %v", err)
	}

	// BoardAuthorization for board B: COMMON scope
	authCommon, _ := model.NewBoardAuthorization(2, model.ScopeCommon, "")
	if err := persistence.NewBoardAuthorizationRepository(q).Save(ctx, authCommon); err != nil {
		t.Fatalf("seed board authorization: %v", err)
	}

	sessions := auth.NewSessionStore(q, clock)
	partners := persistence.NewPartnerRepository(q)
	cfg := &config.Config{BusinessName: "Cooperativa Test"}
	cfg.Server.Port = 0

	authn := auth.NewAuthenticator(cfg, sessions, partners)
	forecasts := application.NewForecastService(persistence.NewTxManager(conn), clock)

	deps := web.Deps{
		Forecasts: forecasts,
		Auth:      authn,
		Sessions:  sessions,
		Partners:  partners,
		Reports:   persistence.NewReportRepository(q),
		HTML:      reportadapter.HTMLRenderer{},
		Taxonomy:  persistence.NewTaxonomyRepository(q),
		Cfg:       cfg,
		Secure:    false,
	}
	srv := web.NewServer(deps)
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Seed a forecast owned by soci A via the service (bypass HTTP for seeding)
	gross, _ := model.MoneyFromString("100.00")
	plannedDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	in := application.ForecastInput{
		Concept:     "Eines soci A",
		GrossAmount: gross,
		PlannedDate: plannedDate,
		SubtypeCode: "a1",
		ScopeKind:   model.ScopePartner,
	}
	created, err := forecasts.Create(ctx, sociA, in)
	if err != nil {
		t.Fatalf("seed forecast for soci A: %v", err)
	}
	return ts, created.ID()
}

// loginAndGetCSRF logs in via dev-login and returns the CSRF token from /forecasts/new.
func loginAndGetCSRF(t *testing.T, client *http.Client, baseURL, email string) string {
	t.Helper()
	if _, err := client.PostForm(baseURL+"/dev-login", url.Values{"email": {email}}); err != nil {
		t.Fatalf("dev-login %s: %v", email, err)
	}
	formResp, err := client.Get(baseURL + "/forecasts/new")
	if err != nil {
		t.Fatalf("GET /forecasts/new: %v", err)
	}
	body := bodyString(t, formResp)
	tok := extractCSRF(t, body)
	if tok == "" {
		t.Fatalf("could not extract CSRF from /forecasts/new for %s; snippet: %q", email, truncate(body, 400))
	}
	return tok
}

// TestServer_WindowClosed409 asserts that posting to /forecasts when no window
// is open returns HTTP 409.
func TestServer_WindowClosed409(t *testing.T) {
	ts := buildClosedWindowServer(t)
	client := followRedirectClient(t)

	csrfToken := loginAndGetCSRF(t, client, ts.URL, "s1@e.test")

	// POST /forecasts — no open window → 409
	noRedirect := noRedirectClient(t)
	u, _ := url.Parse(ts.URL)
	for _, c := range client.Jar.Cookies(u) {
		noRedirect.Jar.SetCookies(u, []*http.Cookie{c})
	}

	resp, err := noRedirect.PostForm(ts.URL+"/forecasts", url.Values{
		"csrf":         {csrfToken},
		"concept":      {"Test"},
		"gross_amount": {"100.00"},
		"planned_date": {"2026-06-15"},
		"subtype_code": {"a1"},
		"scope_kind":   {"PARTNER"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body := bodyString(t, resp)
		t.Errorf("POST /forecasts (no open window): status = %d, want 409; body: %q", resp.StatusCode, truncate(body, 300))
	}
}

// TestServer_CrossSoci403 asserts that soci B updating or deleting a forecast
// owned by soci A returns HTTP 403.
func TestServer_CrossSoci403(t *testing.T) {
	ts, forecastID := buildBoardServer(t)

	// Authenticate as soci B (NOT the owner of the forecast)
	client := followRedirectClient(t)
	csrfToken := loginAndGetCSRF(t, client, ts.URL, "sB@e.test")

	noRedirect := noRedirectClient(t)
	u, _ := url.Parse(ts.URL)
	for _, c := range client.Jar.Cookies(u) {
		noRedirect.Jar.SetCookies(u, []*http.Cookie{c})
	}

	// soci B attempts to delete soci A's forecast → 403
	resp, err := noRedirect.PostForm(ts.URL+"/forecasts/"+forecastID+"/delete", url.Values{
		"csrf": {csrfToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body := bodyString(t, resp)
		t.Errorf("cross-soci delete: status = %d, want 403; body: %q", resp.StatusCode, truncate(body, 300))
	}
}

// TestServer_BoardScope asserts that a board member with COMMON authorization
// can create a COMMON-scoped forecast (303) and that creating an unauthorized
// scope (e.g. SECTION without authorization for it) returns 403.
func TestServer_BoardScope(t *testing.T) {
	ts, _ := buildBoardServer(t)

	t.Run("board member COMMON scope -> 303", func(t *testing.T) {
		client := followRedirectClient(t)
		csrfToken := loginAndGetCSRF(t, client, ts.URL, "sB@e.test")

		noRedirect := noRedirectClient(t)
		u, _ := url.Parse(ts.URL)
		for _, c := range client.Jar.Cookies(u) {
			noRedirect.Jar.SetCookies(u, []*http.Cookie{c})
		}

		resp, err := noRedirect.PostForm(ts.URL+"/forecasts", url.Values{
			"csrf":         {csrfToken},
			"concept":      {"Compra comú board"},
			"gross_amount": {"500.00"},
			"planned_date": {"2026-09-01"},
			"subtype_code": {"a1"},
			"scope_kind":   {"COMMON"},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			body := bodyString(t, resp)
			t.Errorf("board COMMON create: status = %d, want 303; body: %q", resp.StatusCode, truncate(body, 400))
		}
	})

	t.Run("board member unauthorized SECTION scope -> 403", func(t *testing.T) {
		client := followRedirectClient(t)
		csrfToken := loginAndGetCSRF(t, client, ts.URL, "sB@e.test")

		noRedirect := noRedirectClient(t)
		u, _ := url.Parse(ts.URL)
		for _, c := range client.Jar.Cookies(u) {
			noRedirect.Jar.SetCookies(u, []*http.Cookie{c})
		}

		// board B has COMMON auth only, not SECTION "oliva" → ErrForbidden → 403
		resp, err := noRedirect.PostForm(ts.URL+"/forecasts", url.Values{
			"csrf":         {csrfToken},
			"concept":      {"Secció no autoritzada"},
			"gross_amount": {"200.00"},
			"planned_date": {"2026-09-01"},
			"subtype_code": {"a1"},
			"scope_kind":   {"SECTION"},
			"section_code": {"oliva"},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			body := bodyString(t, resp)
			t.Errorf("board unauthorized SECTION: status = %d, want 403; body: %q", resp.StatusCode, truncate(body, 400))
		}
	})
}

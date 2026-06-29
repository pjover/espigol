package report

import (
	"strings"
	"testing"
)

func TestHTMLRenderer_StructureAndNumbers(t *testing.T) {
	rd := buildGolden(t)
	html := string(HTMLRenderer{}.Render(rd))

	for _, want := range []string{
		"<h2>Despesa corrent</h2>",
		"<h2>Despesa d&#39;inversió</h2>", // html-escaped apostrophe (html.EscapeString)
		"<h2>Resum</h2>",
		"<table",
		"2.880,00 €", "23.498,96 €", "11.203,04 €",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q", want)
		}
	}
}

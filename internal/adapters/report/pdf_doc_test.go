package report

import (
	"bytes"
	"testing"
)

func TestRenderDocument_ProducesPDFBytes(t *testing.T) {
	blocks := []Block{
		SectionTitle{Text: "Secció"},
		Table{Title: "Taula", Headers: []string{"A", "B"}, Widths: []uint{8, 4}, Rows: []Row{
			{Cells: []string{"x", "1,00 €"}},
			{Cells: []string{"Total", "1,00 €"}, Bold: true},
			{Cells: []string{"Ajust", "-1,00 €"}, Red: true},
		}},
		PageBreak{},
	}
	out, err := renderDocument("Títol", "29/06/2026", "Cooperativa d'Estellencs", "/nonexistent/logo.png", blocks)
	if err != nil {
		t.Fatalf("renderDocument: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty PDF output")
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("output is not a PDF (no %%PDF header): %.8q", out)
	}
}

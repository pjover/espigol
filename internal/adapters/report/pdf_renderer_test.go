package report

import (
	"bytes"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/ports"
)

func TestPDFRenderer_SmokeOnGolden(t *testing.T) {
	var r ports.ReportRenderer = PDFRenderer{BusinessName: "Cooperativa d'Estellencs", LogoPath: ""}
	out, err := r.Render(buildGolden(t), time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out) == 0 || !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("expected non-empty PDF bytes, got %d bytes prefix %.8q", len(out), out)
	}
}

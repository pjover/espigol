package report

import (
	"testing"
	"time"

	reportmodel "github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

func TestNoopRenderer_ReturnsEmpty(t *testing.T) {
	var r ports.ReportRenderer = NoopRenderer{}
	out, err := r.Render(reportmodel.ReportData{Year: 2026}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("want empty non-nil []byte, got %v (len %d)", out, len(out))
	}
}

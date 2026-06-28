package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

const fixture = "../../../testdata/legacy-espigol.db"

func TestRead_RealFixture(t *testing.T) {
	if _, err := os.Stat(fixture); err != nil {
		t.Skip("legacy fixture not present; skipping (see testdata/)")
	}
	// Copy to a temp path so the test never mutates the fixture.
	src, _ := os.ReadFile(fixture)
	tmp := filepath.Join(t.TempDir(), "legacy.db")
	if err := os.WriteFile(tmp, src, 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(d.Partners) != 8 {
		t.Errorf("partners = %d, want 8", len(d.Partners))
	}
	if len(d.Forecasts) != 35 {
		t.Errorf("forecasts = %d, want 35", len(d.Forecasts))
	}
	if len(d.Types) != 3 || len(d.Subtypes) != 13 {
		t.Errorf("taxonomy: types=%d subtypes=%d, want 3/13", len(d.Types), len(d.Subtypes))
	}
	if len(d.Windows) != 1 || len(d.Reports) != 1 {
		t.Errorf("windows=%d reports=%d, want 1/1", len(d.Windows), len(d.Reports))
	}
	if len(d.Audits) != 61 {
		t.Errorf("audits = %d, want 61", len(d.Audits))
	}
	// money exactness: the two former-REAL values survive as strings.
	var found1322 bool
	for _, f := range d.Forecasts {
		if f.GrossAmount == "1322.22" {
			found1322 = true
		}
	}
	if !found1322 {
		t.Error("expected a forecast with gross 1322.22 read exactly")
	}
}

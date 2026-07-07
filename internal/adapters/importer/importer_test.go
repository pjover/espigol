package importer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/domain/model"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "2025-forecasts.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validBody = `{
  "year": 2025,
  "forecasts": [
    { "partnerId": 7, "scope": "COMMON",  "sectionCode": "",      "subtypeCode": "a1", "concept": "Assegurança", "description": "", "grossAmount": "2880.00", "plannedDate": "2025-06-15" },
    { "partnerId": 1, "scope": "SECTION", "sectionCode": "oliva", "subtypeCode": "a1", "concept": "Poda",       "description": "", "grossAmount": "1200.00", "plannedDate": "2025-03-01" }
  ]
}`

func TestLoad_Valid(t *testing.T) {
	entries, err := importer.Load(writeFile(t, validBody), 2025)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].Scope != model.ScopeCommon || entries[0].PartnerID != 7 {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[1].Scope != model.ScopeSection || entries[1].SectionCode != "oliva" {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if entries[0].GrossAmount.String() != "2880.00" {
		t.Errorf("gross = %s, want 2880.00", entries[0].GrossAmount.String())
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := importer.Load(filepath.Join(t.TempDir(), "nope.json"), 2025)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_YearMismatch(t *testing.T) {
	_, err := importer.Load(writeFile(t, validBody), 2024)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("want year-mismatch error, got %v", err)
	}
}

func TestLoad_BadFields(t *testing.T) {
	cases := map[string]string{
		"bad money": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"abc","plannedDate":"2025-06-15"}]}`,
		"bad date":  `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"nope"}]}`,
		"bad scope": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"WAT","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
		"section no code": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"SECTION","sectionCode":"","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
		"common with code": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","sectionCode":"oliva","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := importer.Load(writeFile(t, body), 2025); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

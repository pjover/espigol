package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestLoadReconciliation_ParsesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reconciliation-2025.json")
	os.WriteFile(path, []byte(`{
      "year": 2025,
      "concessions": [
        {"groupCode":"A6-02","subtypeCode":"a6","concept":"Adob orgànic",
         "requestedTotal":"13880.00","grantedAmount":"13880.00","forecastIds":["CP25008","CP25009"]}
      ],
      "invoices": [
        {"issuer":"Ribot","nif":"B999","number":"FD-39521","issueDate":"2025-03-14",
         "netAmount":"500.00","filePath":"x.pdf","notes":"n",
         "payments":[{"paidOn":"2025-04-01","amount":"500.00"}],
         "links":[{"forecastId":"CP25030","amount":"500.00"}]}
      ]
    }`), 0o644)

	in, err := importer.LoadReconciliation(path, 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	if len(in.Concessions) != 1 || in.Concessions[0].GroupCode != "A6-02" {
		t.Fatalf("concessions: %+v", in.Concessions)
	}
	if in.Concessions[0].GrantedAmount.Cmp(model.MoneyOf(13880)) != 0 {
		t.Errorf("granted = %s", in.Concessions[0].GrantedAmount)
	}
	if len(in.Concessions[0].ForecastIDs) != 2 {
		t.Errorf("forecastIds = %v", in.Concessions[0].ForecastIDs)
	}
	if len(in.Invoices) != 1 || len(in.Invoices[0].Payments) != 1 || len(in.Invoices[0].Links) != 1 {
		t.Fatalf("invoices: %+v", in.Invoices)
	}
}

func TestLoadReconciliation_YearMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reconciliation-2025.json")
	os.WriteFile(path, []byte(`{"year":2024,"concessions":[],"invoices":[]}`), 0o644)
	if _, err := importer.LoadReconciliation(path, 2025); err == nil {
		t.Fatal("expected year-mismatch error")
	}
}

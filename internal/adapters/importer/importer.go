// Package importer reads forecast import files (Home/import/<year>-forecasts.json)
// into application.ForecastImportEntry values. It performs format and scope
// consistency validation only; referential integrity (partner/subtype/section
// existence and the OPEN-window rule) is enforced by ForecastService.AdminImport.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

type fileDoc struct {
	Year      int         `json:"year"`
	Forecasts []fileEntry `json:"forecasts"`
}

type fileEntry struct {
	PartnerID   int    `json:"partnerId"`
	Scope       string `json:"scope"`
	SectionCode string `json:"sectionCode"`
	SubtypeCode string `json:"subtypeCode"`
	Concept     string `json:"concept"`
	Description string `json:"description"`
	GrossAmount string `json:"grossAmount"`
	PlannedDate string `json:"plannedDate"`
}

// Load reads and parses the import file at path, requiring its top-level year to
// equal year. Errors are row-referenced (forecast[i]: ...).
func Load(path string, year int) ([]application.ForecastImportEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading import file: %w", err)
	}
	var doc fileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing import file: %w", err)
	}
	if doc.Year != year {
		return nil, fmt.Errorf("file year %d does not match selected year %d", doc.Year, year)
	}
	entries := make([]application.ForecastImportEntry, 0, len(doc.Forecasts))
	for i, fe := range doc.Forecasts {
		scope, err := model.ParseScopeKind(fe.Scope)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: %w", i, err)
		}
		// Validate scope/sectionCode consistency now for a row-referenced error.
		if _, err := model.NewScope(scope, fe.SectionCode); err != nil {
			return nil, fmt.Errorf("forecast[%d]: %w", i, err)
		}
		gross, err := model.MoneyFromString(fe.GrossAmount)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: invalid grossAmount %q: %w", i, fe.GrossAmount, err)
		}
		planned, err := time.Parse("2006-01-02", fe.PlannedDate)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: invalid plannedDate %q: %w", i, fe.PlannedDate, err)
		}
		entries = append(entries, application.ForecastImportEntry{
			PartnerID:   fe.PartnerID,
			Scope:       scope,
			SectionCode: fe.SectionCode,
			SubtypeCode: fe.SubtypeCode,
			Concept:     fe.Concept,
			Description: fe.Description,
			GrossAmount: gross,
			PlannedDate: planned,
		})
	}
	return entries, nil
}

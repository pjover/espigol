// Package application orchestrates the window lifecycle over the domain ports
// and the pure allocation service.
package application

import (
	"encoding/json"

	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/services"
)

// SnapshotToJSON serializes a computed ReportData to the JSON stored on a Report row.
func SnapshotToJSON(rd report.ReportData) (string, error) {
	b, err := json.Marshal(rd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SnapshotFromJSON deserializes a stored snapshot back into ReportData.
func SnapshotFromJSON(s string) (report.ReportData, error) {
	var rd report.ReportData
	if err := json.Unmarshal([]byte(s), &rd); err != nil {
		return report.ReportData{}, err
	}
	return rd, nil
}

// ReconciliationSnapshotToJSON serializes ReconciliationData for storage.
func ReconciliationSnapshotToJSON(rd services.ReconciliationData) (string, error) {
	b, err := json.Marshal(rd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ReconciliationSnapshotFromJSON deserializes a stored reconciliation snapshot.
func ReconciliationSnapshotFromJSON(s string) (services.ReconciliationData, error) {
	var rd services.ReconciliationData
	if err := json.Unmarshal([]byte(s), &rd); err != nil {
		return services.ReconciliationData{}, err
	}
	return rd, nil
}

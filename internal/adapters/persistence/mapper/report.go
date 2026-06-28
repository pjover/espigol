package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ReportToInsert(r model.Report) sqlc.InsertReportParams {
	return sqlc.InsertReportParams{
		Year:         int64(r.Year()),
		GeneratedAt:  FormatTimestamp(r.GeneratedAt()),
		SnapshotJson: r.SnapshotJSON(),
		Pdf:          r.Pdf(),
		SupersededAt: FormatNullableTimestamp(r.SupersededAt()),
	}
}

func ReportFromRow(r sqlc.Report) (model.Report, error) {
	generatedAt, err := ParseTimestamp(r.GeneratedAt)
	if err != nil {
		return model.Report{}, err
	}
	supersededAt, err := ParseNullableTimestamp(r.SupersededAt)
	if err != nil {
		return model.Report{}, err
	}
	return model.NewReport(int(r.ID), int(r.Year), generatedAt, r.SnapshotJson, r.Pdf, supersededAt)
}

package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

// ReportService provides read-only access to allocation reports: a live
// preview computed from current forecasts, and the latest stored snapshot.
type ReportService struct{ tx ports.TxManager }

func NewReportService(tx ports.TxManager) *ReportService { return &ReportService{tx: tx} }

// Preview computes the live allocation for a DRAFT/OPEN year (nothing stored).
func (s *ReportService) Preview(ctx context.Context, year int) (report.ReportData, error) {
	var rd report.ReportData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		rd, err = computeReportData(ctx, r, w)
		return err
	})
	return rd, err
}

// Latest returns the most recent stored Report snapshot for a year.
func (s *ReportService) Latest(ctx context.Context, year int) (model.Report, bool, error) {
	var rep model.Report
	var found bool
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		rep, found, err = r.Reports.FindLatestByYear(ctx, year)
		return err
	})
	return rep, found, err
}

package application

import (
	"context"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

func itoa(i int) string { return strconv.Itoa(i) }

// adminAudit records an admin mutation with the administrator as actor.
func adminAudit(ctx context.Context, r ports.RepoSet, adminEmail string, kind model.AuditKind, entityType, entityID string, at time.Time) error {
	e, err := model.NewAuditEvent(0, nil, adminEmail, kind, entityType, entityID, at, nil)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}

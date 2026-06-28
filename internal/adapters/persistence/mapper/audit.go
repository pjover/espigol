package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func nullableInt(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullableString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func AuditToInsert(e model.AuditEvent) sqlc.InsertAuditEventParams {
	return sqlc.InsertAuditEventParams{
		ActorID:    nullableInt(e.ActorID()),
		ActorEmail: e.ActorEmail(),
		Kind:       string(e.Kind()),
		EntityType: e.EntityType(),
		EntityID:   e.EntityID(),
		Timestamp:  FormatTimestamp(e.Timestamp()),
		Payload:    nullableString(e.Payload()),
	}
}

func AuditFromRow(r sqlc.AuditEvent) (model.AuditEvent, error) {
	ts, err := ParseTimestamp(r.Timestamp)
	if err != nil {
		return model.AuditEvent{}, err
	}
	kind, err := model.ParseAuditKind(r.Kind)
	if err != nil {
		return model.AuditEvent{}, err
	}
	var actorID *int
	if r.ActorID.Valid {
		v := int(r.ActorID.Int64)
		actorID = &v
	}
	var payload *string
	if r.Payload.Valid {
		payload = &r.Payload.String
	}
	return model.NewAuditEvent(int(r.ID), actorID, r.ActorEmail, kind,
		r.EntityType, r.EntityID, ts, payload)
}

package model

import (
	"fmt"
	"time"
)

type AuditEvent struct {
	id         int
	actorID    *int
	actorEmail string
	kind       AuditKind
	entityType string
	entityID   string
	timestamp  time.Time
	payload    *string
}

func NewAuditEvent(id int, actorID *int, actorEmail string, kind AuditKind,
	entityType, entityID string, timestamp time.Time, payload *string) (AuditEvent, error) {
	if actorEmail == "" {
		return AuditEvent{}, fmt.Errorf("actorEmail must not be empty")
	}
	if _, err := ParseAuditKind(string(kind)); err != nil {
		return AuditEvent{}, err
	}
	return AuditEvent{id, actorID, actorEmail, kind, entityType, entityID, timestamp, payload}, nil
}

func (e AuditEvent) ID() int            { return e.id }
func (e AuditEvent) ActorID() *int      { return e.actorID }
func (e AuditEvent) ActorEmail() string { return e.actorEmail }
func (e AuditEvent) Kind() AuditKind    { return e.kind }
func (e AuditEvent) EntityType() string { return e.entityType }
func (e AuditEvent) EntityID() string   { return e.entityID }
func (e AuditEvent) Timestamp() time.Time { return e.timestamp }
func (e AuditEvent) Payload() *string    { return e.payload }

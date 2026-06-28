package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestAuditLog_AppendAndList(t *testing.T) {
	repo := persistence.NewAuditLog(openTestDB(t))
	ctx := context.Background()

	payload := `{"imported":1}`
	e, _ := model.NewAuditEvent(0, nil, "system@espigol", model.AuditMigration,
		"Partner", "1", time.Date(2026, 6, 23, 15, 15, 59, 0, time.UTC), &payload)
	if err := repo.Append(ctx, e); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("List: len=%d err=%v", len(all), err)
	}
	if all[0].ActorID() != nil {
		t.Errorf("ActorID should be nil, got %v", all[0].ActorID())
	}
	if all[0].Payload() == nil || *all[0].Payload() != payload {
		t.Errorf("payload round trip wrong: %v", all[0].Payload())
	}
}

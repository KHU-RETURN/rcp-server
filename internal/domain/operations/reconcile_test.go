package operations

import (
	"context"
	"github.com/KHU-RETURN/rcp-server/ent/resourceoperation"
	"testing"
)

func TestProviderFailureDoesNotEraseObservation(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	b := NewLiveBackend(db, nil, "")
	if err := b.observe(ctx, Observation{Kind: "instance", ResourceID: "vm", Status: "ACTIVE", Consistency: "matched"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.Reconcile(ctx); err == nil {
		t.Fatal("expected provider failure")
	}
	row := db.ResourceObservation.Query().Where().AllX(ctx)
	for _, r := range row {
		if r.Kind == "instance" && r.Status != "ACTIVE" {
			t.Fatal("provider failure erased last observation")
		}
	}
	if db.ResourceOperation.Query().Where(resourceoperation.Kind("instance.delete")).CountX(ctx) != 0 {
		t.Fatal("provider failure scheduled destructive repair")
	}
}

package store

import (
	"context"
	"testing"
)

func TestWidgetConfigLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.GetActiveWidgetConfig(ctx, "product"); err == nil {
		t.Fatal("expected missing active config to return an error")
	}

	v1, err := st.PublishWidgetConfig(ctx, "product", `{"theme":{"accent":"#111111"}}`)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	if v1.Version != 1 || !v1.Active {
		t.Fatalf("unexpected v1: %+v", v1)
	}

	v2, err := st.PublishWidgetConfig(ctx, "product", `{"theme":{"accent":"#222222"}}`)
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	if v2.Version != 2 || !v2.Active {
		t.Fatalf("unexpected v2: %+v", v2)
	}

	active, err := st.GetActiveWidgetConfig(ctx, "product")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active.Version != 2 || active.Payload != v2.Payload {
		t.Fatalf("expected v2 active, got %+v", active)
	}

	versions, err := st.ListWidgetConfigVersions(ctx, "product")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Version != 2 || versions[1].Version != 1 {
		t.Fatalf("unexpected versions: %+v", versions)
	}

	if err := st.SetActiveWidgetConfigVersion(ctx, "product", 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	active, err = st.GetActiveWidgetConfig(ctx, "product")
	if err != nil {
		t.Fatalf("get active after rollback: %v", err)
	}
	if active.Version != 1 || active.Payload != v1.Payload {
		t.Fatalf("expected v1 active, got %+v", active)
	}
}

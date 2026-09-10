package store

import (
	"context"
	"testing"
	"time"
)

func TestExportDirtyLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, dirty, err := s.ExportDirtySince(ctx); err != nil || dirty {
		t.Fatalf("fresh store dirty = %v, err = %v; want clean", dirty, err)
	}

	if err := s.MarkExportDirty(ctx); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}
	at, dirty, err := s.ExportDirtySince(ctx)
	if err != nil || !dirty || at.IsZero() {
		t.Fatalf("after mark: at=%v dirty=%v err=%v", at, dirty, err)
	}

	if err := s.MarkExportPublished(ctx, at); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	if _, dirty, err := s.ExportDirtySince(ctx); err != nil || dirty {
		t.Fatalf("after publish dirty = %v, err = %v; want clean", dirty, err)
	}
}

// A mutation racing with an in-flight export must leave the store dirty: the
// export is published "up to" the dirty timestamp it observed, not "now".
func TestExportDirtySurvivesRaceWithPublish(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.MarkExportDirty(ctx); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}
	observed, _, err := s.ExportDirtySince(ctx)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}

	time.Sleep(5 * time.Millisecond) // ensure a later timestamp
	if err := s.MarkExportDirty(ctx); err != nil {
		t.Fatalf("second mark: %v", err)
	}
	if err := s.MarkExportPublished(ctx, observed); err != nil {
		t.Fatalf("publish observed: %v", err)
	}

	if _, dirty, err := s.ExportDirtySince(ctx); err != nil || !dirty {
		t.Fatalf("dirty = %v, err = %v; want still dirty after racing mutation", dirty, err)
	}
}

func TestCurationMutationsMarkExportDirty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedRemapReview(t, s, "wb", "wb-1", "100", "100")

	clean := func(step string) {
		t.Helper()
		at, dirty, err := s.ExportDirtySince(ctx)
		if err != nil || !dirty {
			t.Fatalf("%s did not mark export dirty (dirty=%v err=%v)", step, dirty, err)
		}
		if err := s.MarkExportPublished(ctx, at); err != nil {
			t.Fatalf("reset after %s: %v", step, err)
		}
	}

	var id uint
	if err := s.db.Model(&Review{}).Select("id").First(&id).Error; err != nil {
		t.Fatalf("review id: %v", err)
	}

	if err := s.SetReviewVisibility(ctx, id, "hidden"); err != nil {
		t.Fatalf("visibility: %v", err)
	}
	clean("SetReviewVisibility")

	if err := s.SetReviewPinned(ctx, id, true); err != nil {
		t.Fatalf("pinned: %v", err)
	}
	clean("SetReviewPinned")

	text := "спасибо"
	if err := s.SetReviewReply(ctx, id, &text); err != nil {
		t.Fatalf("reply: %v", err)
	}
	clean("SetReviewReply")

	if _, err := s.RemapSellerArticles(ctx, "wb", map[string]string{"100": "200"}); err != nil {
		t.Fatalf("remap: %v", err)
	}
	clean("RemapSellerArticles")

	// A no-op remap must NOT dirty the export.
	if _, err := s.RemapSellerArticles(ctx, "wb", map[string]string{"100": "200"}); err != nil {
		t.Fatalf("noop remap: %v", err)
	}
	if _, dirty, err := s.ExportDirtySince(ctx); err != nil || dirty {
		t.Fatalf("noop remap dirtied export (dirty=%v err=%v)", dirty, err)
	}
}

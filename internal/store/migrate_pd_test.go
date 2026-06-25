package store

import "testing"

func TestSupplierArticleFromRaw(t *testing.T) {
	raw := `{"productDetails":{"supplierArticle":"ART-1"},"userName":"Анна Котова"}`
	if got := supplierArticleFromRaw(raw); got != "ART-1" {
		t.Errorf("got %q, want ART-1", got)
	}
	if got := supplierArticleFromRaw(`{}`); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if got := supplierArticleFromRaw(``); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

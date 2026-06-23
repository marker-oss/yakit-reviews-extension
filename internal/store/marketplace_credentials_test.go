package store

import (
	"context"
	"testing"
)

func TestMarketplaceCredentialPatchPreservesExistingSecrets(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	enabled := true
	cred, err := st.SaveMarketplaceCredential(ctx, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Enabled:     &enabled,
		Values:      map[string]string{"token": "token-1"},
	})
	if err != nil {
		t.Fatalf("save initial: %v", err)
	}
	if cred.PayloadMap()["token"] != "token-1" {
		t.Fatalf("token not saved: %+v", cred.PayloadMap())
	}

	disabled := false
	cred, err = st.SaveMarketplaceCredential(ctx, MarketplaceCredentialPatch{
		Marketplace: "wb",
		Enabled:     &disabled,
		Values:      map[string]string{"token": ""},
	})
	if err != nil {
		t.Fatalf("save patch: %v", err)
	}
	if cred.Enabled {
		t.Fatalf("expected disabled credential")
	}
	if cred.PayloadMap()["token"] != "token-1" {
		t.Fatalf("empty patch should preserve token: %+v", cred.PayloadMap())
	}

	items, err := st.ListMarketplaceCredentials(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].Marketplace != "wb" {
		t.Fatalf("unexpected list: %+v", items)
	}
}

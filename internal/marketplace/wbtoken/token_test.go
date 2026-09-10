package wbtoken

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func testToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	encode := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(map[string]string{"alg": "none"}) + "." + encode(claims) + ".sig"
}

func TestValidatePersonalTokenMetadata(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour).Unix()
	past := now.Add(-24 * time.Hour).Unix()

	cases := []struct {
		name    string
		token   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "personal account with future exp",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 3, "exp": future})
			},
			wantErr: false,
		},
		{
			name: "base account rejected",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 1, "exp": future})
			},
			wantErr: true,
		},
		{
			name: "test account rejected",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 2, "exp": future})
			},
			wantErr: true,
		},
		{
			name: "service account rejected",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 4, "exp": future})
			},
			wantErr: true,
		},
		{
			name: "unknown account rejected",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 99, "exp": future})
			},
			wantErr: true,
		},
		{
			name: "missing acc",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"exp": future})
			},
			wantErr: true,
		},
		{
			name: "non-numeric acc",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": "three", "exp": future})
			},
			wantErr: true,
		},
		{
			name: "missing exp",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 3})
			},
			wantErr: true,
		},
		{
			name: "non-numeric exp",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 3, "exp": "soon"})
			},
			wantErr: true,
		},
		{
			name: "expired exp",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 3, "exp": past})
			},
			wantErr: true,
		},
		{
			name: "one segment",
			token: func(t *testing.T) string {
				return "onlyheader"
			},
			wantErr: true,
		},
		{
			name: "two segments",
			token: func(t *testing.T) string {
				parts := strings.SplitN(testToken(t, map[string]any{"acc": 3, "exp": future}), ".", 3)
				return parts[0] + "." + parts[1]
			},
			wantErr: true,
		},
		{
			name: "four segments",
			token: func(t *testing.T) string {
				return testToken(t, map[string]any{"acc": 3, "exp": future}) + ".extra"
			},
			wantErr: true,
		},
		{
			name: "invalid base64url payload",
			token: func(t *testing.T) string {
				return "header.not-valid-base64!!!.sig"
			},
			wantErr: true,
		},
		{
			name: "invalid json payload",
			token: func(t *testing.T) string {
				encode := func(v any) string {
					raw, _ := json.Marshal(v)
					return base64.RawURLEncoding.EncodeToString(raw)
				}
				badJSON := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
				return encode(map[string]string{"alg": "none"}) + "." + badJSON + ".sig"
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := tc.token(t)
			err := ValidatePersonalTokenMetadata(token, now)
			if tc.wantErr {
				if !errors.Is(err, ErrPersonalTokenRequired) {
					t.Fatalf("expected ErrPersonalTokenRequired, got %v", err)
				}
				if strings.Contains(err.Error(), token) {
					t.Fatalf("error must not contain the source token: %v", err)
				}
				for _, claimValue := range []string{"future", "past", "three", "soon", "not-json"} {
					if strings.Contains(err.Error(), claimValue) {
						t.Fatalf("error must not contain decoded claim values: %v", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

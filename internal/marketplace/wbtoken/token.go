// Package wbtoken validates WB API token metadata (account type and
// expiry) without verifying the JWT signature. WB does not publish a public
// key for personal API tokens, so signature verification is out of scope;
// this package only rejects tokens that are structurally not a personal
// (acc=3), unexpired token.
package wbtoken

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrPersonalTokenRequired is returned for any token that is not a
// well-formed, unexpired, personal (acc=3) WB API token. It never carries
// the source token or decoded claim values.
var ErrPersonalTokenRequired = errors.New("нужен персональный WB API-токен с категорией «Отзывы и вопросы»")

// personalAccountType is the WB JWT "acc" claim value for personal tokens
// scoped to the "Отзывы и вопросы" category.
const personalAccountType = "3"

type tokenClaims struct {
	Account json.Number `json:"acc"`
	Expires json.Number `json:"exp"`
}

// ValidatePersonalTokenMetadata checks that token is a three-segment JWT
// whose payload declares a personal account (acc=3) with an exp timestamp
// after now. It does not verify the JWT signature.
func ValidatePersonalTokenMetadata(token string, now time.Time) error {
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return ErrPersonalTokenRequired
	}
	for _, segment := range segments {
		if segment == "" {
			return ErrPersonalTokenRequired
		}
	}

	raw, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return ErrPersonalTokenRequired
	}

	var claims tokenClaims
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return ErrPersonalTokenRequired
	}

	if claims.Account.String() != personalAccountType {
		return ErrPersonalTokenRequired
	}

	exp, err := claims.Expires.Int64()
	if err != nil {
		return ErrPersonalTokenRequired
	}
	if !time.Unix(exp, 0).After(now) {
		return ErrPersonalTokenRequired
	}

	return nil
}

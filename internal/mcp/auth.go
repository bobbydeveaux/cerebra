package mcp

// auth.go implements the token scheme for the multi-tenant HTTP transport.
//
// A token is a signed assertion of a user_id. The operator (AgentOps) mints
// tokens out-of-band with the same shared secret; this package only validates
// and extracts. The scheme is deliberately simple because the operator is the
// trust boundary: there is one shared secret, set via CEREBRA_TOKEN_SECRET, and
// a token proves "the holder was issued an identity by the operator".
//
// Token format:
//
//	base64url(user_id) "." base64url(HMAC-SHA256(user_id, secret))
//
// Both halves use raw (unpadded) base64url so the token is URL-safe and can
// travel as a ?token= query parameter as well as a Bearer header.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

var (
	// ErrMalformedToken is returned when the token is not in the expected
	// "<payload>.<sig>" shape or either half is not valid base64url.
	ErrMalformedToken = errors.New("malformed token")
	// ErrBadSignature is returned when the HMAC does not match. Tampered
	// tokens and tokens signed with a different secret both land here.
	ErrBadSignature = errors.New("invalid token signature")
	// ErrEmptyUser is returned when the signed payload decodes to an empty
	// user id. An empty user id must never be allowed to resolve to a store.
	ErrEmptyUser = errors.New("empty user id in token")
	// ErrEmptySecret is returned when SignToken/VerifyToken is called with an
	// empty secret. The HTTP transport must fail closed before this happens.
	ErrEmptySecret = errors.New("token secret is empty")
)

var b64 = base64.RawURLEncoding

// SignToken mints a token for userID using secret. It is the inverse of
// VerifyToken and exists primarily so tests (and any future in-process minting)
// can produce valid tokens without duplicating the wire format.
func SignToken(userID, secret string) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	if userID == "" {
		return "", ErrEmptyUser
	}
	sig := sign(userID, secret)
	return b64.EncodeToString([]byte(userID)) + "." + b64.EncodeToString(sig), nil
}

// VerifyToken validates token against secret and returns the asserted user id.
//
// It returns a non-nil error for every failure mode (malformed, tampered,
// wrong secret, empty user, empty secret) and an empty user id alongside it.
// Callers MUST treat any error as "deny, zero data" and MUST NOT fall back to a
// default user. The HMAC comparison is constant-time.
func VerifyToken(token, secret string) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}

	dot := strings.IndexByte(token, '.')
	if dot < 0 {
		return "", ErrMalformedToken
	}
	payloadPart, sigPart := token[:dot], token[dot+1:]

	userBytes, err := b64.DecodeString(payloadPart)
	if err != nil {
		return "", ErrMalformedToken
	}
	gotSig, err := b64.DecodeString(sigPart)
	if err != nil {
		return "", ErrMalformedToken
	}

	userID := string(userBytes)
	if userID == "" {
		return "", ErrEmptyUser
	}

	wantSig := sign(userID, secret)
	if !hmac.Equal(gotSig, wantSig) {
		return "", ErrBadSignature
	}

	return userID, nil
}

func sign(userID, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	return mac.Sum(nil)
}

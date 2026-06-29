package mcp

import (
	"strings"
	"testing"
)

const testSecret = "test-secret-please-rotate"

func TestSignVerifyRoundTrip(t *testing.T) {
	tok, err := SignToken("user-alice", testSecret)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	got, err := VerifyToken(tok, testSecret)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if got != "user-alice" {
		t.Errorf("user id = %q, want user-alice", got)
	}
}

func TestSignTokenRejectsEmptyInputs(t *testing.T) {
	if _, err := SignToken("alice", ""); err != ErrEmptySecret {
		t.Errorf("empty secret err = %v, want ErrEmptySecret", err)
	}
	if _, err := SignToken("", testSecret); err != ErrEmptyUser {
		t.Errorf("empty user err = %v, want ErrEmptyUser", err)
	}
}

func TestVerifyTokenRejectsTampered(t *testing.T) {
	tok, err := SignToken("user-bob", testSecret)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	// Flip the last character of the signature half.
	dot := strings.IndexByte(tok, '.')
	if dot < 0 {
		t.Fatalf("token has no separator: %q", tok)
	}
	body := []byte(tok)
	last := len(body) - 1
	if body[last] == 'A' {
		body[last] = 'B'
	} else {
		body[last] = 'A'
	}
	tampered := string(body)

	if _, err := VerifyToken(tampered, testSecret); err == nil {
		t.Fatal("tampered token verified; expected rejection")
	}
}

func TestVerifyTokenRejectsTamperedPayload(t *testing.T) {
	// A token whose payload claims a different user than the signature covers
	// must be rejected. We forge "user-evil" with bob's signature.
	bob, _ := SignToken("user-bob", testSecret)
	sig := bob[strings.IndexByte(bob, '.'):]
	forged := b64.EncodeToString([]byte("user-evil")) + sig

	if _, err := VerifyToken(forged, testSecret); err != ErrBadSignature {
		t.Errorf("forged-payload err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyTokenRejectsWrongSecret(t *testing.T) {
	tok, _ := SignToken("user-carol", testSecret)
	if _, err := VerifyToken(tok, "a-different-secret"); err != ErrBadSignature {
		t.Errorf("wrong-secret err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyTokenRejectsMalformed(t *testing.T) {
	cases := []string{
		"",            // empty
		"no-dot-here", // missing separator
		"!!!.!!!",     // invalid base64url
		b64.EncodeToString([]byte("alice")) + ".", // empty sig half
		"." + b64.EncodeToString([]byte("sig")),   // empty payload half
	}
	for _, tc := range cases {
		if _, err := VerifyToken(tc, testSecret); err == nil {
			t.Errorf("VerifyToken(%q) accepted; want rejection", tc)
		}
	}
}

func TestVerifyTokenRejectsEmptyUserPayload(t *testing.T) {
	// A correctly-signed token over an empty user id must still be refused, so
	// an empty id can never resolve to a store.
	empty := b64.EncodeToString([]byte(""))
	sig := sign("", testSecret)
	tok := empty + "." + b64.EncodeToString(sig)
	if _, err := VerifyToken(tok, testSecret); err != ErrMalformedToken && err != ErrEmptyUser {
		t.Errorf("empty-user err = %v, want malformed or empty-user rejection", err)
	}
}

func TestVerifyTokenEmptySecretFailsClosed(t *testing.T) {
	tok, _ := SignToken("alice", testSecret)
	if _, err := VerifyToken(tok, ""); err != ErrEmptySecret {
		t.Errorf("empty-secret verify err = %v, want ErrEmptySecret", err)
	}
}

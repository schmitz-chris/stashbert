package auth_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const pw = "korrekt pferd batterie"
	encoded, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || strings.Join(parts[:4], "$") != "$argon2id$v=19$m=19456,t=2,p=1" {
		t.Fatalf("hash = %q, want PHC format with argon2id, v=19, m=19456, t=2, p=1", encoded)
	}
	for i, want := range map[int]int{4: 16, 5: 32} {
		b, err := base64.RawStdEncoding.DecodeString(parts[i])
		if err != nil || len(b) != want {
			t.Errorf("part %d = %q decodes to %d bytes (err %v), want %d", i, parts[i], len(b), err, want)
		}
	}

	ok, err := auth.VerifyPassword(pw, encoded)
	if err != nil || !ok {
		t.Errorf("VerifyPassword(correct) = %v, %v; want true, nil", ok, err)
	}
	ok, err = auth.VerifyPassword("korrekt pferd batteriE", encoded)
	if err != nil || ok {
		t.Errorf("VerifyPassword(wrong) = %v, %v; want false, nil", ok, err)
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	a, err := auth.HashPassword("geheim123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := auth.HashPassword("geheim123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Errorf("two hashes of the same password are equal: %q", a)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	valid, err := auth.HashPassword("geheim123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	parts := strings.Split(valid, "$")
	salt, key := parts[4], parts[5]

	for name, encoded := range map[string]string{
		"empty":             "",
		"argon2i":           "$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key,
		"other version":     "$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$" + key,
		"zero passes":       "$argon2id$v=19$m=19456,t=0,p=1$" + salt + "$" + key,
		"zero threads":      "$argon2id$v=19$m=19456,t=2,p=0$" + salt + "$" + key,
		"trailing garbage":  "$argon2id$v=19$m=19456,t=2,p=1x$" + salt + "$" + key,
		"missing part":      "$argon2id$v=19$m=19456,t=2,p=1$" + salt,
		"invalid base64":    "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$!!!",
		"empty key":         "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$",
		"empty salt":        "$argon2id$v=19$m=19456,t=2,p=1$$" + key,
		"additional fields": valid + "$x",
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := auth.VerifyPassword("geheim123", encoded)
			if err == nil || ok {
				t.Errorf("VerifyPassword(%q) = %v, %v; want false and an error", encoded, ok, err)
			}
		})
	}
}

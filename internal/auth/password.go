// Package auth handles the household password (ADR-0009, architecture.md 8).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters from architecture.md, 8.
const (
	argonMemory  = 19456 // KiB, 19 MiB
	argonTime    = 2
	argonThreads = 1
	saltLen      = 16
	keyLen       = 32
)

var errMalformedHash = errors.New("malformed argon2id hash")

// b64 is the base64 variant of the PHC string format: standard alphabet, no padding.
var b64 = base64.RawStdEncoding

// HashPassword hashes pw with argon2id and a random salt. The result is in
// PHC format: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$%s$%s$%s",
		argon2.Version, formatParams(argonMemory, argonTime, argonThreads), b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches encoded, a hash from HashPassword.
// The parameters are read from encoded. The hashes are compared in constant time.
func VerifyPassword(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errMalformedHash
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil ||
		parts[3] != formatParams(memory, passes, threads) || passes < 1 || threads < 1 {
		return false, errMalformedHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, errMalformedHash
	}
	key, err := b64.DecodeString(parts[5])
	// An empty key would match every password.
	if err != nil || len(key) == 0 {
		return false, errMalformedHash
	}
	other := argon2.IDKey([]byte(pw), salt, passes, memory, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

func formatParams(memory, passes uint32, threads uint8) string {
	return fmt.Sprintf("m=%d,t=%d,p=%d", memory, passes, threads)
}

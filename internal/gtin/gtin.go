// Package gtin validates and normalizes product barcodes (architecture.md, 7.1).
package gtin

import (
	"errors"
	"fmt"
)

// ErrInvalid is returned by Normalize for codes that are not a valid EAN-8,
// UPC-A, EAN-13 or GTIN-14.
var ErrInvalid = errors.New("invalid gtin")

// Normalize validates code and returns its canonical form.
//
// Only ASCII digits are accepted, the length must be 8, 12, 13 or 14 and the
// check digit must be correct. 12-digit codes (UPC-A) get a leading "0",
// 14-digit codes with a leading "0" lose it; both result in 13 digits. All
// other valid codes are returned unchanged. 8-digit codes are checked as
// EAN-8; UPC-E is not supported.
func Normalize(code string) (string, error) {
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return "", fmt.Errorf("%w: %q contains a non-digit", ErrInvalid, code)
		}
	}
	switch len(code) {
	case 8, 12, 13, 14:
	default:
		return "", fmt.Errorf("%w: %q has %d digits, want 8, 12, 13 or 14", ErrInvalid, code, len(code))
	}
	if !validCheckDigit(code) {
		return "", fmt.Errorf("%w: %q has a wrong check digit", ErrInvalid, code)
	}
	switch {
	case len(code) == 12:
		return "0" + code, nil
	case len(code) == 14 && code[0] == '0':
		return code[1:], nil
	}
	return code, nil
}

// IsLocal reports whether normalized is a store-internal number: 13 digits
// with prefix "02" or "20" to "29". Such codes are never looked up externally.
func IsLocal(normalized string) bool {
	if len(normalized) != 13 {
		return false
	}
	if normalized[0] == '0' && normalized[1] == '2' {
		return true
	}
	return normalized[0] == '2' && normalized[1] >= '0' && normalized[1] <= '9'
}

// validCheckDigit reports whether the last digit of code (at least one ASCII
// digit) matches the Modulo 10 check digit of the others. Weights alternate
// 3 and 1, starting with 3 next to the check digit, so leading zeros do not
// change the result.
func validCheckDigit(code string) bool {
	last := len(code) - 1
	sum, weight := 0, 3
	for i := last - 1; i >= 0; i-- {
		sum += int(code[i]-'0') * weight
		weight = 4 - weight
	}
	return (10-sum%10)%10 == int(code[last]-'0')
}

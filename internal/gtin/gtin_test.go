package gtin_test

import (
	"errors"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/gtin"
)

// Check digits: Modulo 10 with weights 3 and 1, starting with 3 at the digit
// left of the check digit. Check digit = (10 - sum mod 10) mod 10.

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		want  string
		local bool
	}{
		{"EAN-13", "3017620422003", "3017620422003", false},
		{"EAN-13", "4001686301265", "4001686301265", false},
		{"UPC-A gets leading zero", "034000470693", "0034000470693", false},
		{"GTIN-14 loses leading zero", "00034000470693", "0034000470693", false},
		{"EAN-8", "20004002", "20004002", false},
		// Prefix 22 (store-internal number with embedded price or weight).
		// Body 221234567890, digits from the right with weights 3,1,3,1,...:
		// 0*3 + 9*1 + 8*3 + 7*1 + 6*3 + 5*1 + 4*3 + 3*1 + 2*3 + 1*1 + 2*3 + 2*1
		// = 0 + 9 + 24 + 7 + 18 + 5 + 12 + 3 + 6 + 1 + 6 + 2 = 93.
		// Check digit = (10 - 93 mod 10) mod 10 = 7, so the code is 2212345678907.
		{"EAN-13 prefix 22", "2212345678907", "2212345678907", true},
		// Prefix 29, body 290000000000: 9*3 + 2*1 = 29 (all other digits are 0).
		// Check digit = (10 - 29 mod 10) mod 10 = 1.
		{"EAN-13 prefix 29", "2900000000001", "2900000000001", true},
		// UPC-A number system 2 becomes prefix 02. Body 21234567890:
		// 0*3 + 9*1 + 8*3 + 7*1 + 6*3 + 5*1 + 4*3 + 3*1 + 2*3 + 1*1 + 2*3
		// = 0 + 9 + 24 + 7 + 18 + 5 + 12 + 3 + 6 + 1 + 6 = 91.
		// Check digit = (10 - 91 mod 10) mod 10 = 9.
		{"UPC-A prefix 02", "212345678909", "0212345678909", true},
		// GTIN-14 with indicator 1 stays unchanged. Body 1301762042200 is the
		// body of 3017620422003 (sum 57) with a leading 1 at weight 3:
		// 57 + 1*3 = 60, check digit = (10 - 60 mod 10) mod 10 = 0.
		{"GTIN-14 without leading zero", "13017620422000", "13017620422000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name+" "+tt.code, func(t *testing.T) {
			got, err := gtin.Normalize(tt.code)
			if err != nil {
				t.Fatalf("Normalize(%q): %v", tt.code, err)
			}
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.code, got, tt.want)
			}
			if again, err := gtin.Normalize(got); err != nil || again != got {
				t.Errorf("Normalize(%q) = %q, %v, want %q (idempotent)", got, again, err, got)
			}
			if local := gtin.IsLocal(got); local != tt.local {
				t.Errorf("IsLocal(%q) = %v, want %v", got, local, tt.local)
			}
		})
	}
}

func TestNormalizeInvalid(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		// Body 400040013015: 5*3 + 1*1 + 0*3 + 3*1 + 1*3 + 0*1 + 0*3 + 4*1
		// + 0*3 + 0*1 + 0*3 + 4*1 = 30, so the check digit must be 0, not 7.
		{"wrong check digit", "4000400130157"},
		{"letters", "abcdefghijklm"},
		{"letter in digits", "301762042200A"},
		{"10 digits", "3017620422"},
		{"empty", ""},
		{"7 digits", "2000400"},
		{"15 digits", "030176204220030"},
		{"space", " 3017620422003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gtin.Normalize(tt.code)
			if !errors.Is(err, gtin.ErrInvalid) {
				t.Errorf("Normalize(%q) error = %v, want ErrInvalid", tt.code, err)
			}
			if got != "" {
				t.Errorf("Normalize(%q) = %q, want empty string", tt.code, got)
			}
		})
	}
}

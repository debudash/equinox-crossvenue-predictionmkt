package card

import (
	"fmt"
	"strings"
)

// Decimal is an exact decimal number held in canonical text form: no leading
// zeros in the integer part, no trailing zeros in the fraction, no "-0".
// "3.50" and "3.5" parse to the same Decimal, so == on Decimal is numeric
// equality. It is used for cutoff_value and other non-price numbers.
type Decimal struct{ s string }

// ParseDecimal accepts an optional leading "-", digits, and an optional
// fraction: "3.5", "03.50", "-0.25", "100". Exponents, "+", spaces, commas,
// percent signs and a bare "." are rejected; fetchers strip units first.
func ParseDecimal(s string) (Decimal, error) {
	neg, intPart, frac, err := splitDecimal(s)
	if err != nil {
		return Decimal{}, err
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	frac = strings.TrimRight(frac, "0")
	out := intPart
	if frac != "" {
		out += "." + frac
	}
	if neg && out != "0" {
		out = "-" + out
	}
	return Decimal{s: out}, nil
}

// String returns the canonical form, e.g. "3.5".
func (d Decimal) String() string { return d.s }

// IsZero reports whether d is the zero value (never set by ParseDecimal).
func (d Decimal) IsZero() bool { return d.s == "" }

func splitDecimal(s string) (neg bool, intPart, frac string, err error) {
	orig := s
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	}
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" || (hasDot && frac == "") || !allDigits(intPart) || !allDigits(frac) {
		return false, "", "", fmt.Errorf("not a decimal: %q", orig)
	}
	return neg, intPart, frac, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Micros is an exact amount of dollars in integer millionths. Prices and fees
// are always Micros, never binary floating point (AGENTS.md hard rule 5).
type Micros int64

// OneDollar is 1 dollar, the payout of one winning contract on either venue.
const OneDollar Micros = 1_000_000

// ParseMicros parses a dollar decimal string such as "0.53" into Micros.
// More than six fractional digits is an error unless the extra digits are zeros,
// so a value is never rounded.
func ParseMicros(s string) (Micros, error) {
	neg, intPart, frac, err := splitDecimal(s)
	if err != nil {
		return 0, err
	}
	frac = strings.TrimRight(frac, "0")
	if len(frac) > 6 {
		return 0, fmt.Errorf("more precision than one millionth of a dollar: %q", s)
	}
	intPart = strings.TrimLeft(intPart, "0")
	if len(intPart) > 12 {
		return 0, fmt.Errorf("amount too large: %q", s)
	}
	var m int64
	for _, c := range intPart + frac + strings.Repeat("0", 6-len(frac)) {
		m = m*10 + int64(c-'0')
	}
	if neg {
		m = -m
	}
	return Micros(m), nil
}

// String returns the canonical dollar decimal, e.g. 530000 -> "0.53".
func (m Micros) String() string {
	sign := ""
	v := int64(m)
	if v < 0 {
		sign, v = "-", -v
	}
	whole, frac := v/1_000_000, v%1_000_000
	if frac == 0 {
		return fmt.Sprintf("%s%d", sign, whole)
	}
	return strings.TrimRight(fmt.Sprintf("%s%d.%06d", sign, whole, frac), "0")
}

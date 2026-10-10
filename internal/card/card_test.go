package card

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestEmptyNeverEqualsAnything(t *testing.T) {
	if None[string]().Equal(None[string]()) {
		t.Error("empty must not equal empty")
	}
	if None[string]().Equal(Some("")) || Some("").Equal(None[string]()) {
		t.Error("empty must not equal a set value")
	}
	if !Some("x").Equal(Some("x")) {
		t.Error("equal set values must be equal")
	}
	if Some("x").Equal(Some("y")) {
		t.Error("different set values must differ")
	}
	var zero Opt[Decimal]
	if !zero.IsEmpty() {
		t.Error("zero Opt must be empty")
	}
}

func TestDecimalNormalizes(t *testing.T) {
	cases := map[string]string{
		"3.50": "3.5", "3.5": "3.5", "03.500": "3.5", "4.00": "4", "100": "100",
		"0.25": "0.25", "0": "0", "-0.0": "0", "-1.10": "-1.1", "000": "0",
	}
	for in, want := range cases {
		d, err := ParseDecimal(in)
		if err != nil || d.String() != want {
			t.Errorf("ParseDecimal(%q) = %q, %v; want %q", in, d, err, want)
		}
	}
	a, _ := ParseDecimal("3.50")
	b, _ := ParseDecimal("3.5")
	if !Some(a).Equal(Some(b)) {
		t.Error("3.50 and 3.5 must be equal")
	}
	for _, bad := range []string{"", ".", ".5", "5.", "3.5%", "1e3", "+1", " 1", "1,000", "abc", "--1"} {
		if _, err := ParseDecimal(bad); err == nil {
			t.Errorf("ParseDecimal(%q) must fail", bad)
		}
	}
}

func TestMicrosExact(t *testing.T) {
	cases := map[string]Micros{
		"0.53": 530_000, "1": 1_000_000, "0": 0, "0.000001": 1, "0.1000000": 100_000,
		"0.07": 70_000, "12.5": 12_500_000, "-0.01": -10_000,
	}
	for in, want := range cases {
		m, err := ParseMicros(in)
		if err != nil || m != want {
			t.Errorf("ParseMicros(%q) = %d, %v; want %d", in, m, err, want)
		}
	}
	for _, bad := range []string{"0.0000001", "abc", "", "1e-3", "1234567890123"} {
		if _, err := ParseMicros(bad); err == nil {
			t.Errorf("ParseMicros(%q) must fail", bad)
		}
	}
	// Integer arithmetic: 0.1 + 0.2 is exactly 0.3.
	a, _ := ParseMicros("0.1")
	b, _ := ParseMicros("0.2")
	c, _ := ParseMicros("0.3")
	if a+b != c {
		t.Errorf("0.1 + 0.2 = %s, want 0.3", a+b)
	}
	for m, want := range map[Micros]string{530_000: "0.53", 1_000_000: "1", 0: "0", 1: "0.000001", -10_000: "-0.01", 12_500_000: "12.5"} {
		if m.String() != want {
			t.Errorf("Micros(%d).String() = %q, want %q", int64(m), m.String(), want)
		}
	}
}

func TestNoFloatInPackage(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	pat := regexp.MustCompile(`float32|float64|ParseFloat|"math"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := pat.FindIndex(b); loc != nil {
			t.Errorf("%s uses floating point: %q (AGENTS.md hard rule 5)", f, b[loc[0]:loc[1]])
		}
	}
}

func TestOpComplement(t *testing.T) {
	pairs := map[Op]Op{Gt: Lte, Lte: Gt, Gte: Lt, Lt: Gte}
	for a, b := range pairs {
		if a.Complement() != b {
			t.Errorf("%s.Complement() = %s, want %s", a, a.Complement(), b)
		}
		if a.Complement().Complement() != a {
			t.Errorf("complement of %s is not an involution", a)
		}
	}
	if Gt.Complement() == Gte || Gte.Complement() == Lte {
		t.Error("gt and gte are not complements")
	}
	if _, ok := ParseOp("ge"); ok {
		t.Error("ParseOp must reject unknown operators")
	}
}

func TestInstantUTC(t *testing.T) {
	a, err := ParseInstant("2026-12-10T14:00:00-05:00")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseInstant("2026-12-10T19:00:00Z")
	c, _ := ParseInstant("2026-12-10T19:00:00.000Z")
	if a.String() != "2026-12-10T19:00:00Z" || a != b || b != c {
		t.Errorf("instants differ: %s %s %s", a, b, c)
	}
	if _, err := ParseInstant("2026-12-10 19:00"); err == nil {
		t.Error("timestamp without offset must be rejected")
	}
}

func TestPayoffFieldsOrderAndEmpty(t *testing.T) {
	v, _ := ParseDecimal("3.50")
	p := Payoff{Subject: Some("fed_funds_upper_bound"), CutoffOp: Some(Gt), CutoffValue: Some(v)}
	fs := p.Fields()
	names := []string{FieldSubject, FieldCutoffOp, FieldCutoffValue, FieldCutoffUnit, FieldDeadline, FieldDecider, FieldUnclearResult}
	if len(fs) != len(names) {
		t.Fatalf("got %d fields", len(fs))
	}
	for i, f := range fs {
		if f.Name != names[i] {
			t.Errorf("field %d is %s, want %s", i, f.Name, names[i])
		}
	}
	if fs[2].Value != "3.5" || !fs[2].Set || fs[3].Set || fs[3].Value != "" {
		t.Errorf("unexpected fields: %+v", fs)
	}
}

// Supports test 17: a value outside vocab.yaml is stored as empty.
func TestVocabUnknownIsEmpty(t *testing.T) {
	v, err := LoadVocab(filepath.Join("..", "..", "config", "vocab.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.Subject("fed_funds_upper_bound").Get(); !ok || got != "fed_funds_upper_bound" {
		t.Error("listed subject must be found")
	}
	if !v.Decider("federal_reserve").Equal(Some("federal_reserve")) {
		t.Error("listed decider must be found")
	}
	if !v.CutoffUnit("percent").Equal(Some("percent")) {
		t.Error("listed unit must be found")
	}
	for _, o := range []Opt[string]{v.Subject("cpi_yoy"), v.Decider("fomc_twitter"), v.CutoffUnit("bps"), v.Subject(""), v.Subject("Fed_Funds_Upper_Bound")} {
		if !o.IsEmpty() {
			t.Error("value outside the vocabulary must be empty")
		}
	}
	if !ParseUnclearResult("refund").IsEmpty() || ParseUnclearResult("half").IsEmpty() {
		t.Error("unclear_result must accept only its listed values")
	}
}

func TestLoadRepoConfig(t *testing.T) {
	c, err := LoadConfig(filepath.Join("..", "..", "config", "equinox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Venues.Allowed) != 2 || c.Router.PriceMaxAge != 10*time.Minute {
		t.Errorf("router or venues not loaded: %+v", c)
	}
	r := c.Roles()
	if r.Reference == "" || r.Counter == "" || r.Reference == r.Counter {
		t.Errorf("roles not loaded: %+v", r)
	}
	if c.Watch.DiscoveryEvery != 30*time.Minute || c.Watch.PricesEvery != 2*time.Minute || c.Watch.Retention != 24*time.Hour {
		t.Errorf("watch section not loaded: %+v", c.Watch)
	}
	if c.Feed.CacheCheckEvery != 5*time.Second || c.Feed.PageSize != 25 || c.Feed.KeepSnapshots != 2 {
		t.Errorf("feed section not loaded: %+v", c.Feed)
	}
	if c.Storage.SQLitePath == "" {
		t.Error("storage path not loaded")
	}
}

func TestConfigRejectsBadRoles(t *testing.T) {
	bad := []string{
		"venues: {allowed: [a, b]}\nrouter: {reference_venue: a, counter_venue: c, price_max_age: 10m}",
		"venues: {allowed: [a, b]}\nrouter: {reference_venue: a, counter_venue: a, price_max_age: 10m}",
		"venues: {allowed: [a, b]}\nrouter: {reference_venue: a, counter_venue: b}",
		"router: {reference_venue: a, counter_venue: b, price_max_age: 10m}",
	}
	for _, y := range bad {
		if _, err := ParseConfig([]byte(y)); err == nil {
			t.Errorf("config must be rejected:\n%s", y)
		}
	}
}

func TestValidationHelpers(t *testing.T) {
	allowed := []string{"a", "b"}
	ok := PriceRecord{Venue: "a", MarketID: "m", Side: Yes, BuyPrice: Some(Micros(530_000))}
	if err := CheckPriceRecord(ok, allowed); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	cases := map[string]PriceRecord{
		"unknown venue":   {Venue: "z", MarketID: "m", Side: Yes},
		"bad side":        {Venue: "a", MarketID: "m", Side: "maybe"},
		"price above one": {Venue: "a", MarketID: "m", Side: No, BuyPrice: Some(Micros(1_000_001))},
		"negative price":  {Venue: "a", MarketID: "m", Side: No, BuyPrice: Some(Micros(-1))},
		"no market id":    {Venue: "a", Side: No},
	}
	for name, p := range cases {
		if err := CheckPriceRecord(p, allowed); !errors.Is(err, ErrInconsistent) {
			t.Errorf("%s: want ErrInconsistent, got %v", name, err)
		}
	}
	if _, err := CheckDeadline("next Tuesday"); !errors.Is(err, ErrInconsistent) {
		t.Errorf("unparseable deadline: want ErrInconsistent, got %v", err)
	}
	if err := CheckCard(Card{Venue: "z", MarketID: "m"}, allowed); !errors.Is(err, ErrInconsistent) {
		t.Errorf("unknown venue card: want ErrInconsistent, got %v", err)
	}
	if err := CheckPrice(OneDollar); err != nil {
		t.Errorf("1 dollar is in range: %v", err)
	}
}

func TestMarketRef(t *testing.T) {
	r := MarketRef{Venue: "a", MarketID: "X-1:2"}
	got, err := ParseMarketRef(r.String())
	if err != nil || got != r {
		t.Errorf("round trip: %v %v", got, err)
	}
	if _, err := ParseMarketRef("nocolon"); err == nil {
		t.Error("ref without colon must fail")
	}
	if Yes.Other() != No || No.Other() != Yes {
		t.Error("Side.Other")
	}
	if BuyOn("a") != "buy_on_a" {
		t.Error("BuyOn")
	}
}

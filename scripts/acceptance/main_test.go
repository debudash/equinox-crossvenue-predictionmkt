package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestTallyAndRender(t *testing.T) {
	stream := strings.Join([]string{
		`{"Action":"run","Test":"TestReq01_SameDespiteTitles"}`,
		`{"Action":"pass","Test":"TestReq01_SameDespiteTitles"}`,
		`{"Action":"pass","Test":"TestReq11_LowerCostWins"}`,
		`{"Action":"fail","Test":"TestReq11_FeesChangeWinner"}`,
		`{"Action":"fail","Test":"TestReq02_Opposite/sub"}`,
		`{"Action":"pass","Test":"TestReq02_Opposite"}`,
		`{"Action":"pass","Test":"TestReq21_NotARequiredNumber"}`,
		`{"Action":"pass","Test":"TestAmd_W7_CliRefusedUnderLock"}`,
		`{"Action":"pass","Test":"TestAmd_W09_BothSides"}`,
		`{"Action":"output","Output":"not json at all"}`,
		`garbage line`,
	}, "\n")
	res, err := Tally(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		table map[int]status
		n     int
		want  status
	}{
		{res.Req, 1, pass},
		{res.Req, 2, pass},  // subtest result is carried by the parent
		{res.Req, 11, fail}, // one of two tests with the same number failed
		{res.Amd, 7, pass},
		{res.Amd, 9, pass},
	}
	for _, c := range checks {
		if got := c.table[c.n]; got != c.want {
			t.Errorf("test %d: got %q, want %q", c.n, got, c.want)
		}
	}
	if _, ok := res.Req[21]; ok {
		t.Error("number 21 is outside the required range and must be ignored")
	}

	var buf bytes.Buffer
	if Render(&buf, res) {
		t.Error("Render must report not ok when tests are missing or failing")
	}
	out := buf.String()
	for _, want := range []string{"  1   pass", "  3   missing", "  11  fail", "  2 of 20 pass", "  W7   pass", "  2 of 11 pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "\n  ") - 2; got != 31 {
		t.Errorf("want 20 + 11 rows, got %d:\n%s", got, out)
	}
}

func TestRenderAllPass(t *testing.T) {
	res := Results{Req: map[int]status{}, Amd: map[int]status{}}
	for n := 1; n <= requiredCount; n++ {
		res.Req[n] = pass
	}
	for n := 1; n <= amendmentCount; n++ {
		res.Amd[n] = pass
	}
	if !Render(&bytes.Buffer{}, res) {
		t.Error("all pass must render ok")
	}
}

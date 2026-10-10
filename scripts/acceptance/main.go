// Command acceptance runs the required tests by name and prints one row per
// test number.
//
// Naming contract (AGENTS.md "Definition of done"):
//   - Required tests 1 to 20: TestReqNN_Description, e.g. TestReq05_GtVsGteIsNotSame.
//   - Amendment tests W1 to W11: TestAmd_WN_Description, e.g. TestAmd_W7_CliRefusedUnderLock
//     (W07 is also accepted).
//
// Several tests may share a number; the number passes only if all of them pass.
// A number with no test is reported as missing, and missing counts as fail.
// Exit status is 1 unless every required and amendment test passes.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const (
	requiredCount  = 20
	amendmentCount = 11
)

var (
	reqName = regexp.MustCompile(`^TestReq(\d{2})_`)
	amdName = regexp.MustCompile(`^TestAmd_W(\d{1,2})_`)
)

type status string

const (
	pass    status = "pass"
	fail    status = "fail"
	missing status = "missing"
)

// event is the subset of `go test -json` output the runner reads.
type event struct {
	Action string
	Test   string
}

// Results holds the outcome per test number for both tables.
type Results struct {
	Req map[int]status
	Amd map[int]status
}

// Tally reads `go test -json` output and folds top-level test outcomes by number.
func Tally(r io.Reader) (Results, error) {
	res := Results{Req: map[int]status{}, Amd: map[int]status{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil || (e.Action != "pass" && e.Action != "fail") {
			continue
		}
		table, n := classify(res, e.Test)
		if table == nil {
			continue
		}
		if e.Action == "fail" || table[n] == fail {
			table[n] = fail
		} else {
			table[n] = pass
		}
	}
	return res, sc.Err()
}

// classify returns the table and number for a top-level test name, or nil.
// Subtests (names containing "/") are ignored; their parent carries the result.
func classify(res Results, name string) (map[int]status, int) {
	if m := reqName.FindStringSubmatch(name); m != nil && !strings.ContainsRune(name, '/') {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 && n <= requiredCount {
			return res.Req, n
		}
	}
	if m := amdName.FindStringSubmatch(name); m != nil && !strings.ContainsRune(name, '/') {
		n, _ := strconv.Atoi(m[1])
		if n >= 1 && n <= amendmentCount {
			return res.Amd, n
		}
	}
	return nil, 0
}

// Render writes both tables and returns true when every row passed.
func Render(w io.Writer, res Results) bool {
	ok := true
	section := func(title, prefix string, count int, table map[int]status) {
		passed := 0
		fmt.Fprintf(w, "%s\n", title)
		for n := 1; n <= count; n++ {
			s, seen := table[n]
			if !seen {
				s = missing
			}
			if s == pass {
				passed++
			} else {
				ok = false
			}
			fmt.Fprintf(w, "  %s%-3d %s\n", prefix, n, s)
		}
		fmt.Fprintf(w, "  %d of %d pass\n\n", passed, count)
	}
	section("Required tests", "", requiredCount, res.Req)
	section("Amendment tests", "W", amendmentCount, res.Amd)
	return ok
}

func main() {
	cmd := exec.Command("go", "test", "./...", "-json", "-count=1", "-run", `^(TestReq\d{2}_|TestAmd_W\d{1,2}_)`)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(2)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(2)
	}
	res, err := Tally(out)
	// go test exits non-zero when any test fails; the tables report that.
	_ = cmd.Wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(2)
	}
	if !Render(os.Stdout, res) {
		os.Exit(1)
	}
}

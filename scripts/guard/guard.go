// Command guard enforces AGENTS.md hard rules 1 and 2 by inspecting source.
//
//   - internal/match and internal/route: no venue name (any case), no network
//     import, no clock read (time.Now, time.Since, time.Until).
//   - internal/card: no venue name.
//
// Venue names are matched as plain text in every file under the directory,
// including tests and comments. Imports and clock reads are found by parsing
// Go source, so an aliased or dot import of "time" is still caught.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var venueNames = []string{"kalshi", "polymarket"}

var clockReads = map[string]bool{"Now": true, "Since": true, "Until": true}

type rule struct {
	dir                    string
	venues, network, clock bool
}

var rules = []rule{
	{dir: "internal/match", venues: true, network: true, clock: true},
	{dir: "internal/route", venues: true, network: true, clock: true},
	{dir: "internal/card", venues: true},
}

// Violation is one broken rule at one source position.
type Violation struct {
	File string
	Line int
	What string
}

func (v Violation) String() string { return fmt.Sprintf("%s:%d: %s", v.File, v.Line, v.What) }

// Check scans the guarded directories under root. A guarded directory that does
// not exist yet is skipped.
func Check(root string) ([]Violation, error) {
	var out []Violation
	for _, r := range rules {
		dir := filepath.Join(root, filepath.FromSlash(r.dir))
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if r.venues {
				out = append(out, venueHits(rel, src)...)
			}
			if (r.network || r.clock) && strings.HasSuffix(path, ".go") {
				hits, err := codeHits(rel, src, r)
				if err != nil {
					return err
				}
				out = append(out, hits...)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func venueHits(file string, src []byte) []Violation {
	var out []Violation
	for i, line := range strings.Split(string(src), "\n") {
		lower := strings.ToLower(line)
		for _, v := range venueNames {
			if strings.Contains(lower, v) {
				out = append(out, Violation{file, i + 1, "venue name " + strconv.Quote(v)})
			}
		}
	}
	return out
}

func codeHits(file string, src []byte, r rule) ([]Violation, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	var out []Violation
	timeNames := map[string]bool{}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		line := fset.Position(imp.Pos()).Line
		if r.network && (path == "net" || strings.HasPrefix(path, "net/")) {
			out = append(out, Violation{file, line, "network import " + strconv.Quote(path)})
		}
		if path == "time" {
			switch {
			case imp.Name == nil:
				timeNames["time"] = true
			case imp.Name.Name == ".":
				if r.clock {
					out = append(out, Violation{file, line, `dot import of "time" hides clock reads`})
				}
			default:
				timeNames[imp.Name.Name] = true
			}
		}
	}
	if r.clock {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && timeNames[id.Name] && clockReads[sel.Sel.Name] {
				out = append(out, Violation{file, fset.Position(sel.Pos()).Line, "clock read time." + sel.Sel.Name})
			}
			return true
		})
	}
	return out, nil
}

func main() {
	vs, err := Check(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "guard:", err)
		os.Exit(2)
	}
	for _, v := range vs {
		fmt.Println(v)
	}
	if len(vs) > 0 {
		fmt.Printf("guard: %d violation(s); see AGENTS.md hard rules 1 and 2\n", len(vs))
		os.Exit(1)
	}
	fmt.Println("guard: ok")
}

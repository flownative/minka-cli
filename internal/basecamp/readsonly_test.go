package basecamp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The source a run was given is reached in one place alone, where the guard is
// put in front of it: any other part of the run that read it would ask past the
// guard, and a run that asks for a single day writes at the source.
func TestTheSourceOfARunIsReachedOnlyThroughTheGuard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var reached []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, isOne := node.(*ast.SelectorExpr); isOne && selector.Sel.Name == "Source" {
				reached = append(reached, set.Position(selector.Pos()).String())
			}
			return true
		})
	}
	if len(reached) != 1 || !strings.HasPrefix(reached[0], "extract.go:") {
		t.Errorf("the source of a run is reached at %v, and only the guard in extract.go may reach it", reached)
	}
}

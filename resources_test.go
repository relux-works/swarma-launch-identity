package identity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Closed production file/import sets reject added OS/network/process or init
// dependencies. Runtime lookup/write instrumentation complements this source
// control; it is not a general proof against arbitrary hostile source edits.
func TestResourceBoundary(t *testing.T) {
	allowed := map[string]map[string]bool{
		"identity.go":                        {"crypto/sha256": true, "encoding/binary": true, "encoding/hex": true, "errors": true, "unicode": true, "unicode/utf8": true},
		"internal/cli/cli.go":                {"io": true, "github.com/relux-works/swarma-launch-identity": true},
		"cmd/swarma-launch-identity/main.go": {"os": true, "syscall": true, "github.com/relux-works/swarma-launch-identity/internal/cli": true},
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		path = filepath.ToSlash(path)
		imports, ok := allowed[path]
		if !ok {
			t.Fatalf("unreviewed production file: %s", path)
		}
		seen[path] = true
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil || !imports[name] || imp.Name != nil {
				t.Fatal("unadmitted production import")
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "init" {
				t.Fatal("unexpected initialization effect")
			}
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "syscall" && sel.Sel.Name != "Write" && sel.Sel.Name != "Stdout" {
					t.Fatal("unadmitted syscall operation")
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" {
					switch sel.Sel.Name {
					case "Exit", "Args", "LookupEnv", "Stderr":
					default:
						t.Fatal("unadmitted OS operation")
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(allowed) {
		t.Fatal("production boundary files missing")
	}
}

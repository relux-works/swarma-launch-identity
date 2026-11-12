package identity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	entrypointFile = "cmd/swarma-launch-identity/main.go"
	checkFrameFile = "internal/cli/cli.go"
)

// TestResourceBoundary closes production files/imports and ambient environment
// reads except main's single os.LookupEnv seam. Runtime lookup/write
// instrumentation complements this source control; it is not a general proof
// against arbitrary hostile source edits.
func TestResourceBoundary(t *testing.T) {
	allowed := map[string]map[string]bool{
		"identity.go":         {"crypto/sha256": true, "encoding/binary": true, "encoding/hex": true, "errors": true, "unicode": true, "unicode/utf8": true},
		"internal/cli/cli.go": {"io": true, "github.com/relux-works/swarma-launch-identity": true},
		entrypointFile:        {"io": true, "os": true, "syscall": true, "github.com/relux-works/swarma-launch-identity/internal/cli": true},
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
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if path == entrypointFile && admittedEnvironmentLookupSeam(path, f) == nil {
			t.Fatal("missing os.LookupEnv seam in main at cmd/swarma-launch-identity/main.go")
		}
		if pos, found := unadmittedEnvironmentRead(path, f); found {
			t.Fatalf("unadmitted ambient environment read at %s:%d", path, fset.Position(pos).Line)
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
					case "Exit", "Args", "LookupEnv", "Stdin", "Stderr":
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

// admittedEnvironmentLookupSeam names the only permitted ambient read:
// os.LookupEnv passed by main in cmd/swarma-launch-identity/main.go to
// runCommand's lookup parameter.
func admittedEnvironmentLookupSeam(path string, f *ast.File) *ast.SelectorExpr {
	if path != entrypointFile {
		return nil
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" || fn.Body == nil {
			continue
		}
		if len(fn.Body.List) != 1 {
			return nil
		}
		stmt, ok := fn.Body.List[0].(*ast.ExprStmt)
		if !ok {
			return nil
		}
		exitCall, ok := stmt.X.(*ast.CallExpr)
		if !ok || !isPackageCall(exitCall.Fun, "os", "Exit") || len(exitCall.Args) != 1 {
			return nil
		}
		runCall, ok := exitCall.Args[0].(*ast.CallExpr)
		if !ok || !isFunctionCall(runCall.Fun, "runCommand") || len(runCall.Args) != 5 {
			return nil
		}
		lookup, ok := runCall.Args[1].(*ast.SelectorExpr)
		if !ok || !isPackageSelector(lookup, "os", "LookupEnv") {
			return nil
		}
		return lookup
	}
	return nil
}

// TestResourceBoundaryLookupPlant proves the guard admits only main's lookup
// seam and finds ambient reads planted in dispatch and check-frame code.
func TestResourceBoundaryLookupPlant(t *testing.T) {
	path := entrypointFile
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil || admittedEnvironmentLookupSeam(path, file) == nil || hasUnadmittedEnvironmentRead(path, file) {
		t.Fatal("current entrypoint failed its admitted lookup control")
	}
	marker := `if len(args) > 0 && args[0] == "check-frame" {`
	planted := strings.Replace(string(source), marker, marker+"\n\t\t_, _ = os.LookupEnv(\"FORBIDDEN_FIXTURE\")", 1)
	if planted == string(source) {
		t.Fatal("could not plant the dispatcher lookup fixture")
	}
	file, err = parser.ParseFile(token.NewFileSet(), path, planted, 0)
	if err != nil || !hasUnadmittedEnvironmentRead(path, file) {
		t.Fatal("resource guard failed to detect the planted dispatcher lookup")
	}

	checkFrame, err := os.ReadFile(checkFrameFile)
	if err != nil {
		t.Fatal(err)
	}
	checkFrameMarker := "func CheckFrame(args []string, stdin io.Reader, stdout, stderr io.Writer) (exit int) {"
	planted = strings.Replace(string(checkFrame), checkFrameMarker, checkFrameMarker+"\n\t_, _ = os.LookupEnv(\"FORBIDDEN_FIXTURE\")", 1)
	if planted == string(checkFrame) {
		t.Fatal("could not plant the check-frame lookup fixture")
	}
	file, err = parser.ParseFile(token.NewFileSet(), checkFrameFile, planted, 0)
	if err != nil || !hasUnadmittedEnvironmentRead(checkFrameFile, file) {
		t.Fatal("resource guard failed to detect the planted check-frame lookup")
	}
}

func hasUnadmittedEnvironmentRead(path string, f *ast.File) bool {
	_, found := unadmittedEnvironmentRead(path, f)
	return found
}

func unadmittedEnvironmentRead(path string, f *ast.File) (token.Pos, bool) {
	allowed := admittedEnvironmentLookupSeam(path, f)
	var found token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && isAmbientEnvironmentRead(sel) && sel != allowed {
			found = sel.Pos()
			return false
		}
		return true
	})
	return found, found.IsValid()
}

func isAmbientEnvironmentRead(sel *ast.SelectorExpr) bool {
	if isPackageSelector(sel, "os", "Getenv") || isPackageSelector(sel, "os", "LookupEnv") ||
		isPackageSelector(sel, "os", "Environ") || isPackageSelector(sel, "os", "ExpandEnv") {
		return true
	}
	return isPackageSelector(sel, "syscall", "Getenv") || isPackageSelector(sel, "syscall", "Environ")
}

func isPackageCall(expr ast.Expr, pkg, name string) bool {
	call, ok := expr.(*ast.SelectorExpr)
	return ok && isPackageSelector(call, pkg, name)
}

func isPackageSelector(sel *ast.SelectorExpr, pkg, name string) bool {
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func isFunctionCall(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

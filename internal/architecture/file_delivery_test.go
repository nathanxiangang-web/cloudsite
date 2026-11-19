package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// File delivery may resolve a URL, while the dedicated preview service is the
// only caller allowed to read a file source. HTTP delivery cannot proxy bytes.
func TestFileDeliveryHasNoByteTransferPath(t *testing.T) {
	for _, pkg := range []string{"fileaccess", "fileshare", "producthttp"} {
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join("..", pkg)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
					continue
				}
				checkFileDeliveryBoundary(t, filepath.Join(dir, name), pkg == "fileaccess" && name == "preview_service.go")
			}
		})
	}
}

func checkFileDeliveryBoundary(t *testing.T, path string, preview bool) {
	t.Helper()
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		alias := filepath.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = importPath
		if importPath == "net/http/httputil" || alias == "." && (importPath == "io" || importPath == "net/http") {
			t.Errorf("%s: forbidden delivery import %s", positions.Position(spec.Pos()), importPath)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if selector.Sel.Name == "OpenPreview" && !preview {
			t.Errorf("%s: only the preview service may open file bytes", positions.Position(selector.Pos()))
		}
		if receiver, ok := selector.X.(*ast.Ident); ok {
			switch imports[receiver.Name] + "." + selector.Sel.Name {
			case "io.Copy", "io.CopyN", "io.CopyBuffer", "net/http.ServeContent", "net/http.ServeFile", "net/http.ServeFileFS":
				if !preview {
					t.Errorf("%s: file delivery may only redirect", positions.Position(selector.Pos()))
				}
			}
		}
		return true
	})
}

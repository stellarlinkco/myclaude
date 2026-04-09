package wrapper

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppAvoidsLegacyBackendAndLoggerImports(t *testing.T) {
	root := projectRootForApp(t)
	files, err := filepath.Glob(filepath.Join(root, "internal", "app", "*.go"))
	if err != nil {
		t.Fatalf("Glob(): %v", err)
	}

	for _, path := range files {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}

		imports := parseAppImports(t, path)
		for _, imp := range imports {
			switch imp {
			case "codeagent-wrapper/internal/backend", "codeagent-wrapper/internal/logger":
				t.Fatalf("%s imports legacy package %q", base, imp)
			}
		}
	}
}

func parseAppImports(t *testing.T, path string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", path, err)
	}

	imports := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, `"`))
	}
	return imports
}

func projectRootForApp(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(): %v", err)
	}
	return root
}

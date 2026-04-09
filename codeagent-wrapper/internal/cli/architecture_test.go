package cli

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"
)

func TestCLIImportBoundaries(t *testing.T) {
	root := projectRoot(t)

	assertExactImports(t,
		filepath.Join(root, "cmd", "codeagent-wrapper", "main.go"),
		[]string{"codeagent-wrapper/internal/cli"},
	)

	assertExactImports(t,
		filepath.Join(root, "internal", "cli", "cli.go"),
		[]string{"codeagent-wrapper/internal/app"},
	)

	assertForbiddenImports(t,
		filepath.Join(root, "internal", "app", "cli.go"),
		[]string{
			"codeagent-wrapper/internal/backend",
			"codeagent-wrapper/internal/executor",
			"codeagent-wrapper/internal/logger",
		},
	)
}

func assertExactImports(t *testing.T, path string, want []string) {
	t.Helper()

	got := parseImports(t, path)
	if len(got) != len(want) {
		t.Fatalf("%s imports = %v, want %v", path, got, want)
	}
	for _, imp := range want {
		if !slices.Contains(got, imp) {
			t.Fatalf("%s imports = %v, missing %q", path, got, imp)
		}
	}
}

func assertForbiddenImports(t *testing.T, path string, forbidden []string) {
	t.Helper()

	got := parseImports(t, path)
	for _, imp := range forbidden {
		if slices.Contains(got, imp) {
			t.Fatalf("%s imports forbidden package %q", path, imp)
		}
	}
}

func parseImports(t *testing.T, path string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", path, err)
	}

	imports := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		imports = append(imports, imp.Path.Value[1:len(imp.Path.Value)-1])
	}
	return imports
}

func projectRoot(t *testing.T) string {
	t.Helper()

	root := filepath.Clean(filepath.Join("..", ".."))
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err != nil {
			t.Fatalf("Abs(%q): %v", root, err)
		}
		root = abs
	}
	return root
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolScaffoldGo(t *testing.T) {
	dir := t.TempDir()
	cmd := newToolCommand()
	cmd.SetArgs([]string{"scaffold", "my-echo", "--lang", "go", "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold go: %v", err)
	}

	outDir := filepath.Join(dir, "my-echo")
	expectedFiles := []string{
		"guest.go",
		"go.mod",
		"Makefile",
		"tool.yaml",
		"fixtures/test_echo.json",
		"README.md",
	}
	for _, name := range expectedFiles {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("missing file: %s", name)
		}
	}

	guestContent, err := os.ReadFile(filepath.Join(outDir, "guest.go"))
	if err != nil {
		t.Fatalf("read guest.go: %v", err)
	}
	if !strings.Contains(string(guestContent), "contract_version") {
		t.Error("guest.go missing contract_version field")
	}
	if !strings.Contains(string(guestContent), "func main()") {
		t.Error("guest.go missing main function")
	}

	toolContent, err := os.ReadFile(filepath.Join(outDir, "tool.yaml"))
	if err != nil {
		t.Fatalf("read tool.yaml: %v", err)
	}
	if !strings.Contains(string(toolContent), "type: wasm") {
		t.Error("tool.yaml missing type: wasm")
	}
	if !strings.Contains(string(toolContent), "my-echo") {
		t.Error("tool.yaml missing tool name")
	}

	modContent, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(modContent), "module my_echo") {
		t.Errorf("go.mod has unexpected module name: %s", string(modContent))
	}

	makeContent, err := os.ReadFile(filepath.Join(outDir, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makeContent), "GOOS=wasip1") {
		t.Error("Makefile missing GOOS=wasip1")
	}
}

func TestToolScaffoldRust(t *testing.T) {
	dir := t.TempDir()
	cmd := newToolCommand()
	cmd.SetArgs([]string{"scaffold", "my-echo", "--lang", "rust", "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scaffold rust: %v", err)
	}

	outDir := filepath.Join(dir, "my-echo")
	expectedFiles := []string{
		"src/main.rs",
		"Cargo.toml",
		"Makefile",
		"tool.yaml",
		"fixtures/test_echo.json",
		"README.md",
	}
	for _, name := range expectedFiles {
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("missing file: %s", name)
		}
	}

	rsContent, err := os.ReadFile(filepath.Join(outDir, "src/main.rs"))
	if err != nil {
		t.Fatalf("read main.rs: %v", err)
	}
	if !strings.Contains(string(rsContent), "contract_version") {
		t.Error("main.rs missing contract_version")
	}

	cargoContent, err := os.ReadFile(filepath.Join(outDir, "Cargo.toml"))
	if err != nil {
		t.Fatalf("read Cargo.toml: %v", err)
	}
	if !strings.Contains(string(cargoContent), "my_echo") {
		t.Error("Cargo.toml missing crate name")
	}

	makeContent, err := os.ReadFile(filepath.Join(outDir, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(makeContent), "wasm32-wasip1") {
		t.Error("Makefile missing wasm32-wasip1 target")
	}
}

func TestToolScaffoldInvalidLang(t *testing.T) {
	dir := t.TempDir()
	cmd := newToolCommand()
	cmd.SetArgs([]string{"scaffold", "my-echo", "--lang", "python", "--dir", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported language, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("unexpected error: %v", err)
	}
}

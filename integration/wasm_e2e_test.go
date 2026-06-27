package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestWASME2E builds the WASM binary and runs Node.js E2E tests.
//
// Prerequisites: Node.js 18+ available on PATH.
// Skip conditions: -short flag, node not found, node < 18.
func TestWASME2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping WASM E2E in short mode")
	}

	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH; skipping WASM E2E tests")
	}

	nodeVer := nodeMajorVersion(t, nodePath)
	if nodeVer < 18 {
		t.Skipf("node version %d < 18 required; skipping WASM E2E tests", nodeVer)
	}

	// Resolve paths relative to the project root (where go.mod lives).
	projectRoot := findProjectRoot(t)
	wasmDir := filepath.Join(projectRoot, "wasm")
	wasmFile := filepath.Join(wasmDir, "md4go.wasm")
	wasmExecFile := filepath.Join(wasmDir, "wasm_exec.js")
	testFile := filepath.Join(wasmDir, "md4go_e2e.test.js")

	// Step 1: Build WASM binary.
	t.Log("building WASM binary ...")
	buildCmd := exec.Command("go", "build", "-o", wasmFile, "./wasm")
	buildCmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	buildCmd.Dir = projectRoot
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("WASM build failed:\n%s", string(buildOut))
	}
	t.Logf("WASM binary built: %s (%d bytes)", wasmFile, fileSize(wasmFile))

	// Step 2: Ensure wasm_exec.js exists.
	if _, err := os.Stat(wasmExecFile); os.IsNotExist(err) {
		goroot := runtime.GOROOT()
		src := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js")
		t.Logf("copying wasm_exec.js from %s ...", src)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("failed to read wasm_exec.js from GOROOT: %v", err)
		}
		if err := os.WriteFile(wasmExecFile, data, 0644); err != nil {
			t.Fatalf("failed to write wasm_exec.js: %v", err)
		}
	}

	// Step 3: Run Node.js tests.
	testFlag := "--test"
	if nodeVer < 20 {
		testFlag = "--experimental-test-runner" // Node 18-19
	}
	// spec reporter gives clean, readable output.
	args := []string{testFlag, "--test-reporter", "spec", testFile}
	t.Logf("running: node %s", strings.Join(args, " "))

	cmd := exec.Command(nodePath, args...)
	cmd.Dir = wasmDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	// Always log output for diagnostics.
	if stdout.Len() > 0 {
		t.Logf("node stdout:\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Logf("node stderr:\n%s", stderr.String())
	}
	if runErr != nil {
		t.Fatalf("WASM E2E tests failed: %v", runErr)
	}
	t.Log("WASM E2E tests PASSED")
}

// nodeMajorVersion returns the major version of Node.js (e.g. 20 for v20.11.0).
func nodeMajorVersion(t *testing.T, nodePath string) int {
	t.Helper()
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return 0
	}
	ver := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	major, _, _ := strings.Cut(ver, ".")
	n, _ := strconv.Atoi(major)
	return n
}

// findProjectRoot walks up from the test file's directory to find go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find project root (go.mod not found)")
		}
		dir = parent
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

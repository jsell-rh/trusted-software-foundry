package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimalSpec = `apiVersion: foundry/v1
kind: Application
metadata:
  name: watch-test
  version: 1.0.0
components:
  foundry-http: v1.0.0
`

func TestWatchCmd_MissingOutputFlag(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "app.foundry.yaml")
	if err := os.WriteFile(specPath, []byte(minimalSpec), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd()
	cmd.SetArgs([]string{"watch", specPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --output flag, got nil")
	}
	if !strings.Contains(err.Error(), "--output") {
		t.Errorf("expected '--output' in error, got: %v", err)
	}
}

func TestWatchCmd_MissingSpecFile(t *testing.T) {
	tmp := t.TempDir()
	outDir := filepath.Join(tmp, "out")

	cmd := rootCmd()
	cmd.SetArgs([]string{"watch", "/nonexistent/app.foundry.yaml", "--output", outDir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing spec file, got nil")
	}
	if !strings.Contains(err.Error(), "spec file") {
		t.Errorf("expected 'spec file' in error, got: %v", err)
	}
}

func TestWatchCmd_NoArgs(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"watch"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing spec argument, got nil")
	}
}

func TestWatchCmd_InitialCompile(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "app.foundry.yaml")
	if err := os.WriteFile(specPath, []byte(minimalSpec), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	// Use a very short interval so the test exits quickly after initial compile.
	// We'll cancel via the spec file being unreadable (or just kill after first tick).
	// Simplest: use --interval with a context-cancel approach by running in goroutine.
	done := make(chan error, 1)
	go func() {
		cmd := rootCmd()
		var buf strings.Builder
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"watch", specPath, "--output", outDir, "--interval", "50ms"})
		done <- cmd.Execute()
	}()

	// Wait for the output directory + main.go to appear (initial compile).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(outDir, "main.go")); err == nil {
			// Compiled OK. Now make the spec invalid to cause an error that exits.
			// Actually — we just need to verify initial compile worked.
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("main.go not generated after initial compile within 5s")
}

func TestRecompile_Success(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "app.foundry.yaml")
	if err := os.WriteFile(specPath, []byte(minimalSpec), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	var buf strings.Builder
	if err := recompile(specPath, outDir, "", &buf); err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if !strings.Contains(buf.String(), "Compiled OK") {
		t.Errorf("expected 'Compiled OK' in output, got: %s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "main.go")); os.IsNotExist(err) {
		t.Error("main.go not generated")
	}
}

func TestRecompile_InvalidSpec(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "bad.foundry.yaml")
	if err := os.WriteFile(specPath, []byte("not: valid: yaml: spec\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	var buf strings.Builder
	err := recompile(specPath, outDir, "", &buf)
	if err == nil {
		t.Fatal("expected error for invalid spec, got nil")
	}
}

func TestWatchCmd_IntervalParsing(t *testing.T) {
	// Verify that --interval flag accepts duration strings.
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "app.foundry.yaml")
	if err := os.WriteFile(specPath, []byte(minimalSpec), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")

	// Run with a non-default interval — just test that flag parses correctly
	// by starting the goroutine and verifying no flag-parse error.
	errCh := make(chan error, 1)
	go func() {
		cmd := rootCmd()
		cmd.SetArgs([]string{"watch", specPath, "--output", outDir, "--interval", "200ms"})
		errCh <- cmd.Execute()
	}()

	// Give it time to do at least one compile then stop caring.
	time.Sleep(300 * time.Millisecond)
	// If there was a flag parse error it would come back immediately.
	select {
	case err := <-errCh:
		if err != nil && strings.Contains(err.Error(), "interval") {
			t.Errorf("interval flag error: %v", err)
		}
	default:
		// Still running — that's fine (watcher is waiting for signal).
	}
}

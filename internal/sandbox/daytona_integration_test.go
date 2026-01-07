// Package sandbox provides integration tests for the Daytona backend.
// Run with: DAYTONA_API_KEY=xxx go test -v ./internal/sandbox -run TestDaytona
package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// skipIfNoAPIKey skips the test if the Daytona API key is not set.
func skipIfNoAPIKey(t *testing.T) {
	if os.Getenv("DAYTONA_API_KEY") == "" {
		t.Skip("DAYTONA_API_KEY not set, skipping integration test")
	}
}

// TestDaytonaBackendAvailable tests that the Daytona API is accessible.
func TestDaytonaBackendAvailable(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	if !backend.IsAvailable() {
		t.Fatal("Daytona backend is not available - check API key")
	}
	t.Log("Daytona backend is available")
}

// TestDaytonaSandboxLifecycle tests the full sandbox lifecycle.
func TestDaytonaSandboxLifecycle(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-test-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s (sandbox ID: %s)", session.ID, session.Metadata[MetaSandboxID])

	// Ensure cleanup
	defer func() {
		t.Log("Destroying sandbox...")
		if err := backend.Destroy(context.Background(), session); err != nil {
			t.Errorf("Failed to destroy sandbox: %v", err)
		} else {
			t.Log("Sandbox destroyed")
		}
	}()

	// Check if session exists
	exists, err := backend.HasSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("Failed to check session: %v", err)
	}
	if !exists {
		t.Fatal("Session should exist after creation")
	}
	t.Log("Session exists")

	// Test that IsRunning returns false before Start
	running, err := backend.IsRunning(ctx, session)
	if err != nil {
		t.Fatalf("Failed to check running state: %v", err)
	}
	t.Logf("IsRunning before Start: %v", running)
}

// TestDaytonaFileSync tests file upload and download.
func TestDaytonaFileSync(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for file sync test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-filesync-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "gastown-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file
	testContent := "Hello from GasTown integration test!\nLine 2\nLine 3"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create subdirectory with file
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	subFile := filepath.Join(subDir, "nested.txt")
	if err := os.WriteFile(subFile, []byte("Nested content"), 0644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	// Upload to sandbox
	t.Log("Uploading files to sandbox...")
	remotePath := "/home/daytona/uploaded"
	if err := backend.SyncToSession(ctx, session, tmpDir, remotePath); err != nil {
		t.Fatalf("Failed to sync to session: %v", err)
	}
	t.Log("Files uploaded successfully")

	// Download back to different location
	t.Log("Downloading files from sandbox...")
	downloadDir, err := os.MkdirTemp("", "gastown-download-*")
	if err != nil {
		t.Fatalf("Failed to create download dir: %v", err)
	}
	defer os.RemoveAll(downloadDir)

	if err := backend.SyncFromSession(ctx, session, remotePath, downloadDir); err != nil {
		t.Fatalf("Failed to sync from session: %v", err)
	}
	t.Log("Files downloaded successfully")

	// Verify downloaded content
	downloadedFile := filepath.Join(downloadDir, "test.txt")
	content, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), testContent)
	} else {
		t.Log("File content verified successfully")
	}
}

// TestDaytonaPtySession tests PTY session with command execution.
func TestDaytonaPtySession(t *testing.T) {
	skipIfNoAPIKey(t)

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for PTY test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-pty-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Start a simple shell command instead of Claude (faster test)
	t.Log("Starting PTY session with bash...")
	if err := backend.Start(ctx, session, "bash"); err != nil {
		t.Fatalf("Failed to start PTY session: %v", err)
	}
	t.Log("PTY session started")

	// Wait a moment for bash to initialize
	time.Sleep(2 * time.Second)

	// Send a command
	t.Log("Sending 'echo hello' command...")
	if err := backend.SendInput(ctx, session, "echo hello"); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Wait for output
	time.Sleep(2 * time.Second)

	// Capture output
	output, err := backend.CaptureOutput(ctx, session, 50)
	if err != nil {
		t.Fatalf("Failed to capture output: %v", err)
	}
	t.Logf("Captured output (%d bytes):\n%s", len(output), output)

	// Verify output contains "hello"
	if !strings.Contains(output, "hello") {
		t.Errorf("Output should contain 'hello', got: %s", output)
	} else {
		t.Log("Output verified - contains 'hello'")
	}

	// Stop the session
	t.Log("Stopping session...")
	if err := backend.Stop(ctx, session); err != nil {
		t.Errorf("Failed to stop session: %v", err)
	} else {
		t.Log("Session stopped")
	}
}

// TestDaytonaClaudeCode tests running actual Claude Code in a sandbox.
// This is a longer test that requires ANTHROPIC_API_KEY.
func TestDaytonaClaudeCode(t *testing.T) {
	skipIfNoAPIKey(t)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Claude Code test")
	}

	backend := NewDaytonaBackend(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Create a sandbox
	t.Log("Creating sandbox for Claude Code test...")
	session, err := backend.Create(ctx, CreateOptions{
		Name:    "gastown-claude-" + time.Now().Format("20060102-150405"),
		WorkDir: "/home/daytona",
	})
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	t.Logf("Created sandbox: %s", session.ID)

	defer func() {
		t.Log("Destroying sandbox...")
		backend.Destroy(context.Background(), session)
	}()

	// Start Claude Code (uses default command)
	t.Log("Starting Claude Code...")
	if err := backend.Start(ctx, session, ""); err != nil {
		t.Fatalf("Failed to start Claude Code: %v", err)
	}
	t.Log("Claude Code started")

	// Wait for Claude to initialize
	t.Log("Waiting for Claude to initialize...")
	time.Sleep(10 * time.Second)

	// Capture initial output
	output, err := backend.CaptureOutput(ctx, session, 100)
	if err != nil {
		t.Logf("Warning: Failed to capture initial output: %v", err)
	} else {
		t.Logf("Initial output (%d bytes):\n%s", len(output), output)
	}

	// Send a simple prompt
	t.Log("Sending prompt to Claude...")
	if err := backend.SendInput(ctx, session, "What is 2+2? Answer with just the number."); err != nil {
		t.Fatalf("Failed to send input: %v", err)
	}

	// Wait for response
	t.Log("Waiting for Claude response...")
	time.Sleep(15 * time.Second)

	// Capture output
	output, err = backend.CaptureOutput(ctx, session, 200)
	if err != nil {
		t.Fatalf("Failed to capture output: %v", err)
	}
	t.Logf("Claude output (%d bytes):\n%s", len(output), output)

	// Stop
	t.Log("Stopping Claude...")
	if err := backend.Stop(ctx, session); err != nil {
		t.Errorf("Failed to stop: %v", err)
	}
	t.Log("Test completed")
}

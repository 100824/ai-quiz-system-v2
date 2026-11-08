package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDuplicatePortDoesNotInitializeDatabase(t *testing.T) {
	if os.Getenv("QUIZ_STARTUP_TEST_HELPER") == "1" {
		main()
		return
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDuplicatePortDoesNotInitializeDatabase$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "QUIZ_STARTUP_TEST_HELPER=1", "PORT="+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil || err == nil || !strings.Contains(string(output), "address already in use") {
		t.Fatalf("expected immediate port conflict: %v, %s", err, output)
	}
	if _, err = os.Stat(filepath.Join(root, "backend-go")); !os.IsNotExist(err) {
		t.Fatalf("duplicate startup touched application data before checking the port: %v", err)
	}
}

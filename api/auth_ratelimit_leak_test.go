package api

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuthRateLimiterDoesNotStartBackgroundGoroutines(t *testing.T) {
	if os.Getenv("ORLOJ_AUTH_RATELIMIT_LEAK_CHILD") == "1" {
		baseline := runtime.NumGoroutine()
		for i := 0; i < 8; i++ {
			_ = newAuthRateLimiter(nil, nil)
		}
		time.Sleep(50 * time.Millisecond)
		fmt.Printf("%d\n", runtime.NumGoroutine()-baseline)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestAuthRateLimiterDoesNotStartBackgroundGoroutines$")
	cmd.Env = append(os.Environ(), "ORLOJ_AUTH_RATELIMIT_LEAK_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("limiter subprocess failed: %v\n%s", err, output)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		t.Fatalf("expected goroutine delta output, got %q", output)
	}
	delta, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("parse goroutine delta failed: %v\n%s", err, output)
	}
	if delta > 0 {
		t.Fatalf("expected no background goroutine growth, got delta %d", delta)
	}
}

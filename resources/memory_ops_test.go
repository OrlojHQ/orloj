package resources

import (
	"strings"
	"testing"
)

func TestNormalizeMemoryOperation(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantOK  bool
	}{
		{"read", MemoryOperationRead, true},
		{"READ", MemoryOperationRead, true},
		{" memory.read ", MemoryOperationRead, true},
		{"write", MemoryOperationWrite, true},
		{"memory.write", MemoryOperationWrite, true},
		{"search", MemoryOperationSearch, true},
		{"list", MemoryOperationList, true},
		{"ingest", MemoryOperationIngest, true},
		{"invalid", "", false},
		{"", "", false},
		{"  ", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeMemoryOperation(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Fatalf("NormalizeMemoryOperation(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestNormalizeMemoryOperations_DedupesAndOrders(t *testing.T) {
	out, err := NormalizeMemoryOperations([]string{"read", "READ", "memory.write", "read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 unique ops, got %v", out)
	}
	if out[0] != MemoryOperationRead || out[1] != MemoryOperationWrite {
		t.Fatalf("unexpected order/content: %v", out)
	}
}

func TestNormalizeMemoryOperations_Invalid(t *testing.T) {
	_, err := NormalizeMemoryOperations([]string{"read", "nope"})
	if err == nil {
		t.Fatal("expected error for invalid operation")
	}
	if !strings.Contains(err.Error(), "invalid memory operation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeMemoryOperations_EmptySlice(t *testing.T) {
	out, err := NormalizeMemoryOperations(nil)
	if err != nil || out != nil {
		t.Fatalf("expected nil,nil got %v %v", out, err)
	}
}

func TestMemoryToolNamesForOperations(t *testing.T) {
	names := MemoryToolNamesForOperations([]string{"read", "list"})
	if len(names) != 2 {
		t.Fatalf("got %v", names)
	}
	if names[0] != "memory.read" || names[1] != "memory.list" {
		t.Fatalf("unexpected names: %v", names)
	}
	if len(MemoryToolNamesForOperations([]string{"bad"})) != 0 {
		t.Fatal("expected empty for invalid ops")
	}
	if len(MemoryToolNamesForOperations(nil)) != 0 {
		t.Fatal("expected empty for nil")
	}
}

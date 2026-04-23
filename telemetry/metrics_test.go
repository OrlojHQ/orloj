package telemetry

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordToolExecution(t *testing.T) {
	RecordToolExecution("test-tool", "wasm", "ok", 0.5)
	RecordToolExecution("test-tool", "wasm", "error", 1.2)
	RecordToolExecution("test-tool", "http", "ok", 0.3)

	count := testutil.CollectAndCount(ToolExecutionDuration)
	if count == 0 {
		t.Error("expected ToolExecutionDuration to have observations")
	}
}

func TestRecordWASMExecution(t *testing.T) {
	RecordWASMExecution("wasm-tool", 500)

	expected := `
	# HELP orloj_wasm_fuel_consumed Total fuel consumed by WASM tool executions.
	# TYPE orloj_wasm_fuel_consumed counter
	orloj_wasm_fuel_consumed{tool="wasm-tool"} 500
	`
	if err := testutil.CollectAndCompare(WASMFuelConsumed, strings.NewReader(expected)); err != nil {
		t.Errorf("WASMFuelConsumed: %v", err)
	}
}

func TestRecordWASMCacheHitMiss(t *testing.T) {
	RecordWASMCacheHit("cache-tool")
	RecordWASMCacheHit("cache-tool")
	RecordWASMCacheMiss("cache-tool")

	hitsExpected := `
	# HELP orloj_wasm_compilation_cache_hits_total Total WASM module compilation cache hits.
	# TYPE orloj_wasm_compilation_cache_hits_total counter
	orloj_wasm_compilation_cache_hits_total{tool="cache-tool"} 2
	`
	if err := testutil.CollectAndCompare(WASMCompilationCacheHits, strings.NewReader(hitsExpected)); err != nil {
		t.Errorf("WASMCompilationCacheHits: %v", err)
	}

	missExpected := `
	# HELP orloj_wasm_compilation_cache_misses_total Total WASM module compilation cache misses.
	# TYPE orloj_wasm_compilation_cache_misses_total counter
	orloj_wasm_compilation_cache_misses_total{tool="cache-tool"} 1
	`
	if err := testutil.CollectAndCompare(WASMCompilationCacheMisses, strings.NewReader(missExpected)); err != nil {
		t.Errorf("WASMCompilationCacheMisses: %v", err)
	}
}

func TestRecordWASMModuleFetch(t *testing.T) {
	RecordWASMModuleFetch("https", 1.5)
	RecordWASMModuleFetch("oci", 2.0)

	count := testutil.CollectAndCount(WASMModuleFetchDuration)
	if count == 0 {
		t.Error("expected WASMModuleFetchDuration to have observations")
	}
}

func TestRecordWASMExecutionZeroFuel(t *testing.T) {
	before := testutil.ToFloat64(WASMFuelConsumed.WithLabelValues("zero-tool"))
	RecordWASMExecution("zero-tool", 0)
	after := testutil.ToFloat64(WASMFuelConsumed.WithLabelValues("zero-tool"))
	if after != before {
		t.Error("RecordWASMExecution should not increment counter when fuel is 0")
	}
}

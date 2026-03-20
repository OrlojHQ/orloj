;; Reference WASI guest module for Orloj wasm tool runtime.
;; It writes a valid contract v1 response envelope to stdout.
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 32) "{\"contract_version\":\"v1\",\"status\":\"ok\",\"output\":\"reference wasm module\"}\n")
  (func (export "run")
    (i32.store (i32.const 0) (i32.const 32))
    (i32.store (i32.const 4) (i32.const 73))
    (call $fd_write
      (i32.const 1)
      (i32.const 0)
      (i32.const 1)
      (i32.const 8))
    drop))

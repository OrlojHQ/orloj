// echo_guest is a WASI guest that implements the Orloj WASM tool contract v1.
// It reads a JSON request from stdin and echoes the input field back as output.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o echo_guest.wasm ./examples/resources/tools/wasm-reference/echo_guest.go
package main

import (
	"encoding/json"
	"io"
	"os"
)

type request struct {
	ContractVersion string `json:"contract_version"`
	Tool            string `json:"tool"`
	Input           string `json:"input"`
}

type response struct {
	ContractVersion string `json:"contract_version"`
	Status          string `json:"status"`
	Output          string `json:"output"`
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		writeError("failed to read stdin: " + err.Error())
		return
	}
	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		writeError("failed to parse request: " + err.Error())
		return
	}
	resp := response{
		ContractVersion: "v1",
		Status:          "ok",
		Output:          req.Input,
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

func writeError(msg string) {
	resp := struct {
		ContractVersion string `json:"contract_version"`
		Status          string `json:"status"`
		Error           struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		ContractVersion: "v1",
		Status:          "error",
	}
	resp.Error.Code = "guest_error"
	resp.Error.Message = msg
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

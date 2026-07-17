//go:build !cgo

package main

import (
	"encoding/json"
	"fmt"
)

// Pure-Go stubs so unit tests can build without a C compiler.
// Production plugin builds use cgo_bridge.go (//go:build cgo).

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {}

// callHost is not available without the CPA host; tests must mock higher-level helpers.
func callHost(method string, payload any) (json.RawMessage, error) {
	_ = payload
	return nil, fmt.Errorf("host callback %s unavailable without cgo", method)
}

// Package ethereum implements the deliberately limited Ethereum read gateway.
package ethereum

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const MaxRequestBytes = 16 << 10
const MaxResponseBytes = 2 << 20

var integerID = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var address = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var hash = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
var quantity = regexp.MustCompile(`^0x(0|[1-9a-fA-F][0-9a-fA-F]{0,63})$`)
var data = regexp.MustCompile(`^0x([0-9a-fA-F]{2})*$`)

type request struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// object rejects duplicate envelope fields and trailing JSON to avoid ambiguity.
func object(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("expected field")
		}
		if _, exists := fields[name]; exists {
			return nil, errors.New("duplicate field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return fields, nil
}

func parse(raw []byte) (request, *rpcError) {
	var req request
	if !json.Valid(raw) {
		return req, &rpcError{-32700, "Parse error"}
	}
	fields, err := object(raw)
	if err != nil {
		return req, &rpcError{-32600, "Invalid Request"}
	}
	for name := range fields {
		if name != "jsonrpc" && name != "id" && name != "method" && name != "params" {
			return req, &rpcError{-32600, "Invalid Request"}
		}
	}
	id := fields["id"]
	var stringID string
	if len(id) == 0 || len(id) > 128 || (!integerID.Match(id) && (id[0] != '"' || json.Unmarshal(id, &stringID) != nil)) {
		return req, &rpcError{-32600, "Invalid Request"}
	}
	req.ID = id
	if json.Unmarshal(fields["jsonrpc"], &req.JSONRPC) != nil || req.JSONRPC != "2.0" || json.Unmarshal(fields["method"], &req.Method) != nil || req.Method == "" {
		return req, &rpcError{-32600, "Invalid Request"}
	}
	if p, ok := fields["params"]; ok {
		if len(p) == 0 || p[0] != '[' || json.Unmarshal(p, &req.Params) != nil {
			return req, &rpcError{-32602, "Invalid params"}
		}
	} else {
		req.Params = []json.RawMessage{}
	}
	if !validParams(req) {
		switch req.Method {
		case "eth_chainId", "eth_blockNumber", "eth_getBalance", "eth_getCode", "eth_getTransactionReceipt":
			return req, &rpcError{-32602, "Invalid params"}
		default:
			return req, &rpcError{-32601, "Method not found"}
		}
	}
	return req, nil
}

func stringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func validParams(req request) bool {
	switch req.Method {
	case "eth_chainId", "eth_blockNumber":
		return len(req.Params) == 0
	case "eth_getBalance", "eth_getCode":
		if len(req.Params) != 2 || !address.MatchString(stringValue(req.Params[0])) {
			return false
		}
		block := stringValue(req.Params[1])
		switch block {
		case "latest", "earliest", "safe", "finalized", "pending":
			return true
		}
		return quantity.MatchString(block)
	case "eth_getTransactionReceipt":
		return len(req.Params) == 1 && hash.MatchString(stringValue(req.Params[0]))
	}
	return false
}

func validResult(method string, result json.RawMessage) bool {
	switch method {
	case "eth_chainId":
		return stringValue(result) == "0x1"
	case "eth_blockNumber", "eth_getBalance":
		return quantity.MatchString(stringValue(result))
	case "eth_getCode":
		return data.MatchString(stringValue(result))
	case "eth_getTransactionReceipt":
		// Receipt fields evolve with Ethereum forks; validate the envelope, not a frozen fork schema.
		if bytes.Equal(result, []byte("null")) {
			return true
		}
		_, err := object(result)
		return err == nil
	}
	return false
}

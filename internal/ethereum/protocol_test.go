package ethereum

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		code int
	}{
		{`{"jsonrpc":"2.0","id":9007199254740993,"method":"eth_blockNumber"}`, 0},
		{`{"jsonrpc":"2.0","id":"a","method":"eth_chainId","params":[]}`, 0},
		{fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"eth_getBalance","params":["0x%s","finalized"]}`, strings.Repeat("a", 40)), 0},
		{`{`, -32700}, {`{} {}`, -32700}, {`[]`, -32600},
		{`{"jsonrpc":"2.0","method":"eth_chainId"}`, -32600},
		{`{"jsonrpc":"2.0","id":null,"method":"eth_chainId"}`, -32600},
		{`{"jsonrpc":"2.0","id":1.2,"method":"eth_chainId"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"id":2,"method":"eth_chainId"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","url":"http://evil"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":null}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[1]}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":[]}`, -32601},
		{`{"jsonrpc":"2.0","id":1,"method":"debug_traceTransaction","params":[]}`, -32601},
		{fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"eth_getCode","params":["0x%s","0x00"]}`, strings.Repeat("a", 40)), -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionReceipt","params":["0x12"]}`, -32602},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			req, err := parse([]byte(tt.raw))
			if tt.code == 0 {
				if err != nil {
					t.Fatal(err)
				}
				if !validParams(req) {
					t.Fatal("invalid accepted request")
				}
				return
			}
			if err == nil || err.Code != tt.code {
				t.Fatalf("got %v want %d", err, tt.code)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > MaxRequestBytes {
			return
		}
		req, err := parse(raw)
		if err == nil && (!json.Valid(req.ID) || !validParams(req)) {
			t.Fatal("accepted invalid request")
		}
	})
}

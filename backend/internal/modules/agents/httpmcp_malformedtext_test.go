// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func postMCP(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("posting: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func TestAnMCPRequestWithBrokenTextIsAParseErrorBeforeAnyToolSeesIt(t *testing.T) {
	_, srv := largeBodyHandler(t, NewRegistry(nil, nil))
	call := func(arg string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whoami","arguments":{"note":"` + arg + `"}}}`
	}
	for name, body := range map[string]string{
		"invalid bytes":     call("bad \xff\xfe"),
		"lone surrogate":    call(`\ud800`),
		"lone low in a key": strings.Replace(call("x"), `"note"`, `"n\udc00"`, 1),
	} {
		status, answer := postMCP(t, srv.URL, body)
		if status != http.StatusOK || !strings.Contains(answer, "-32700") {
			t.Errorf("%s: %d %s, want a -32700 parse error", name, status, answer)
		}
	}
	if _, answer := postMCP(t, srv.URL, call(`fine 😀 é`)); strings.Contains(answer, "-32700") {
		t.Errorf("a valid pair and valid UTF-8 were refused: %s", answer)
	}
}

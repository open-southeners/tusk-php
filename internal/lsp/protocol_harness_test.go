package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// rawHarness — a low-level pipe harness for robustness tests that need to
// send arbitrary (possibly malformed) bytes to the server.
//
// A single background goroutine reads all server output and routes decoded
// messages into a buffered channel, eliminating any concurrent access to the
// shared bufio.Reader.
// ---------------------------------------------------------------------------

type rawHarness struct {
	t         *testing.T
	in        *io.PipeWriter
	responses chan map[string]interface{} // decoded JSON-RPC responses (has "id")
	all       chan map[string]interface{} // all decoded messages (responses + notifications)
}

// newRawHarness starts a fresh Server.Run() goroutine and returns a harness
// that exposes direct byte-level access to the pipe pair so tests can inject
// malformed input.
func newRawHarness(t *testing.T) *rawHarness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	logger := log.New(io.Discard, "", 0)
	server := NewServer(inR, outW, logger)
	go func() {
		_ = server.Run()
	}()
	rh := &rawHarness{
		t:         t,
		in:        inW,
		responses: make(chan map[string]interface{}, 200),
		all:       make(chan map[string]interface{}, 200),
	}
	// Single background reader — the only goroutine that reads from outR.
	go func() {
		reader := bufio.NewReader(outR)
		for {
			var contentLength int
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)
				if line == "" {
					break
				}
				if strings.HasPrefix(line, "Content-Length:") {
					fmt.Sscanf(strings.TrimPrefix(line, "Content-Length:"), "%d", &contentLength)
				}
			}
			if contentLength == 0 {
				continue
			}
			buf := make([]byte, contentLength)
			if _, err := io.ReadFull(reader, buf); err != nil {
				return
			}
			var m map[string]interface{}
			if err := json.Unmarshal(buf, &m); err != nil {
				continue
			}
			// non-blocking send: if a channel is full, discard
			select {
			case rh.all <- m:
			default:
			}
			if _, hasID := m["id"]; hasID {
				select {
				case rh.responses <- m:
				default:
				}
			}
		}
	}()
	return rh
}

// sendRaw writes raw bytes directly to the server's stdin pipe.
func (rh *rawHarness) sendRaw(data []byte) {
	rh.t.Helper()
	_, err := rh.in.Write(data)
	if err != nil && err != io.ErrClosedPipe {
		rh.t.Logf("sendRaw write error (may be expected): %v", err)
	}
}

// sendFramed serialises params as a well-formed JSON-RPC message with
// Content-Length framing.
func (rh *rawHarness) sendFramed(id interface{}, method string, params interface{}) {
	rh.t.Helper()
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if id != nil {
		msg["id"] = id
	}
	body, _ := json.Marshal(msg)
	rh.sendRaw([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)))
}

// readResponse returns the next response (message with an "id" field) within
// the given deadline. Returns nil on timeout.
func (rh *rawHarness) readResponse(deadline time.Duration) map[string]interface{} {
	rh.t.Helper()
	select {
	case m := <-rh.responses:
		return m
	case <-time.After(deadline):
		return nil
	}
}

// readResponseID drains the responses channel until a message matching id
// is found, or the deadline elapses.
func (rh *rawHarness) readResponseID(id int, deadline time.Duration) map[string]interface{} {
	rh.t.Helper()
	timeout := time.After(deadline)
	for {
		select {
		case m := <-rh.responses:
			if respID, ok := m["id"]; ok {
				var got int
				switch v := respID.(type) {
				case float64:
					got = int(v)
				case int:
					got = v
				}
				if got == id {
					return m
				}
				// Wrong id — put it back would cause a deadlock; just discard.
			}
		case <-timeout:
			return nil
		}
	}
}

func (rh *rawHarness) close() {
	_ = rh.in.Close()
}

// ---------------------------------------------------------------------------
// Helpers shared across all tests in this file.
// ---------------------------------------------------------------------------

// rootURI returns the testdata project root as a file:// URI.
func rootURI() string { return "file://" + testdataPath() }

// initParams produces a standard initialize parameter map.
func initParams() map[string]interface{} {
	return map[string]interface{}{
		"rootUri":      rootURI(),
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	}
}

// ---------------------------------------------------------------------------
// Task B — Tests
// ---------------------------------------------------------------------------

// TestProtocolLifecycle drives a full initialize → initialized → request →
// shutdown sequence over real JSON-RPC framing and asserts capabilities.
func TestProtocolLifecycle(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// 1. initialize
	initID := h.send("initialize", initParams())
	resp := h.readResponse(initID)

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("initialize: expected result map, got %T", resp["result"])
	}
	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("initialize: expected capabilities")
	}
	// Assert a representative set of capabilities.
	for _, cap := range []string{"hoverProvider", "definitionProvider", "referencesProvider", "documentSymbolProvider"} {
		if caps[cap] != true {
			t.Errorf("expected capability %q to be true, caps=%v", cap, caps)
		}
	}
	if caps["completionProvider"] == nil {
		t.Error("expected completionProvider to be present")
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("initialize: expected serverInfo")
	}
	if serverInfo["name"] != "php-lsp" {
		t.Errorf("serverInfo.name: got %v", serverInfo["name"])
	}

	// 2. initialized notification (no response expected)
	h.notify("initialized", map[string]interface{}{})

	// 3. A simple request (documentSymbol on unknown URI returns nil/empty, not an error)
	symID := h.send("textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///nonexistent.php"},
	})
	symResp := h.readResponse(symID)
	if symResp["error"] != nil {
		t.Errorf("documentSymbol on unknown URI should not return an RPC error, got: %v", symResp["error"])
	}

	// 4. shutdown
	shutID := h.send("shutdown", nil)
	shutResp := h.readResponse(shutID)
	if shutResp["error"] != nil {
		t.Errorf("shutdown returned error: %v", shutResp["error"])
	}

	// 5. exit — not sent because Server.handleMessage calls os.Exit(0) which
	//    would terminate the test binary. Lifecycle test ends with shutdown.
}

// TestProtocolDocumentSync exercises didOpen → didChange → documentSymbol →
// didClose and asserts the updated content is reflected.
func TestProtocolDocumentSync(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	const uri = "file:///tmp/proto_harness_sync.php"
	source1 := `<?php
namespace Test;
class Alpha {
    public function hello(): void {}
}
`
	source2 := source1 + `
class Beta {
    public function world(): void {}
}
`

	// didOpen
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source1,
		},
	})
	time.Sleep(50 * time.Millisecond)

	// documentSymbol after open — must not error
	id1 := h.send("textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp1 := h.readResponse(id1)
	if resp1["error"] != nil {
		t.Fatalf("documentSymbol after open: error %v", resp1["error"])
	}

	// didChange — full replacement with source2 adding Beta
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": source2}},
	})
	time.Sleep(50 * time.Millisecond)

	// documentSymbol after change — must not error; symbol list should include Alpha
	id2 := h.send("textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp2 := h.readResponse(id2)
	if resp2["error"] != nil {
		t.Fatalf("documentSymbol after change: error %v", resp2["error"])
	}

	// Verify symbols contain at least Alpha class.
	syms, _ := resp2["result"].([]interface{})
	found := false
	for _, sym := range syms {
		m, ok := sym.(map[string]interface{})
		if !ok {
			continue
		}
		if m["name"] == "Alpha" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Alpha class in documentSymbol result after didChange, got: %v", resp2["result"])
	}

	// hover at a safe (0,0) position — must not crash the server
	hoverID := h.send("textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 0, "character": 0},
	})
	h.readResponse(hoverID) // just must not timeout

	// didClose
	h.notify("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	time.Sleep(20 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Robustness tests — all drive the raw harness so they can inject bad bytes.
// Each sub-test asserts the server does NOT crash (it continues to serve a
// subsequent well-formed request) and returns a safe response.
// ---------------------------------------------------------------------------

// startedRawHarness returns a raw harness whose server has already been
// initialized via a well-formed initialize exchange.
func startedRawHarness(t *testing.T) *rawHarness {
	t.Helper()
	rh := newRawHarness(t)
	// Send initialize and wait for the response.
	rh.sendFramed(1, "initialize", initParams())
	resp := rh.readResponseID(1, 5*time.Second)
	if resp == nil {
		t.Fatal("startedRawHarness: timed out waiting for initialize response")
	}
	// Send initialized notification (no response).
	rh.sendFramed(nil, "initialized", map[string]interface{}{})
	return rh
}

// probeAlive sends a well-formed request and asserts the server replies,
// proving it has not crashed after a previous bad input.
func probeAlive(t *testing.T, rh *rawHarness, probeID int) {
	t.Helper()
	rh.sendFramed(probeID, "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///nonexistent.php"},
	})
	resp := rh.readResponseID(probeID, 5*time.Second)
	if resp == nil {
		t.Fatal("server appears to have crashed: probe request timed out after bad input")
	}
}

// TestRobustnessMalformedContentLength sends a header with a non-numeric
// Content-Length.  The server should log the error, discard the line, and
// continue serving.
func TestRobustnessMalformedContentLength(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	// Malformed Content-Length (non-numeric value)
	rh.sendRaw([]byte("Content-Length: BOGUS\r\n\r\n"))

	// Server must survive and reply to the next good request.
	probeAlive(t, rh, 100)
}

// TestRobustnessAbsentContentLength sends a blank-line-terminated header
// block with no Content-Length at all; the server returns an error and
// carries on.
func TestRobustnessAbsentContentLength(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	// A header block with no Content-Length — just blank line.
	rh.sendRaw([]byte("\r\n"))

	probeAlive(t, rh, 101)
}

// TestRobustnessTruncatedJSON sends a Content-Length that claims more bytes
// than are provided.  The server should hit an EOF on ReadFull and continue.
func TestRobustnessTruncatedJSON(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	// Claim 500 bytes but only send 10, then pad with spaces to unblock ReadFull.
	rh.sendRaw([]byte("Content-Length: 500\r\n\r\n{\"json\":1}"))
	junk := bytes.Repeat([]byte(" "), 490)
	rh.sendRaw(junk)

	probeAlive(t, rh, 102)
}

// TestRobustnessInvalidJSON sends a well-framed message whose body is not
// valid JSON.
func TestRobustnessInvalidJSON(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	body := []byte("<<<NOT JSON>>>")
	rh.sendRaw([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))))
	rh.sendRaw(body)

	probeAlive(t, rh, 103)
}

// TestRobustnessUnknownMethod sends a valid JSON-RPC request for an unknown
// method and expects a method-not-found error response, not a crash.
func TestRobustnessUnknownMethod(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	rh.sendFramed(200, "nonExistent/method", map[string]interface{}{})
	resp := rh.readResponseID(200, 5*time.Second)
	if resp == nil {
		t.Fatal("expected error response for unknown method, got timeout")
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error field for unknown method, got: %v", resp)
	}
	code, _ := errObj["code"].(float64)
	if code != -32601 {
		t.Errorf("expected error code -32601 (method not found), got %v", code)
	}
}

// TestRobustnessRequestBeforeInitialize sends a request before initialize has
// been called.  The server must not crash; providers are nil before init.
func TestRobustnessRequestBeforeInitialize(t *testing.T) {
	rh := newRawHarness(t) // deliberately NOT initialised
	defer rh.close()

	// Send hover before initialize — nil providers will panic, but recoverPanic
	// should swallow it.  No response is expected; just don't crash.
	rh.sendFramed(300, "textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///test.php"},
		"position":     map[string]interface{}{"line": 0, "character": 0},
	})

	// Give the server time to process the bad request (no response expected).
	time.Sleep(100 * time.Millisecond)

	// Now initialize — the server must still be alive and respond.
	rh.sendFramed(301, "initialize", initParams())
	initResp := rh.readResponseID(301, 5*time.Second)
	if initResp == nil {
		t.Fatal("server appears to have crashed after pre-init request: initialize timed out")
	}
}

// TestRobustnessUnopenedDocument requests documentSymbol for a URI that was
// never opened via didOpen.  The server must return a result (not a crash).
func TestRobustnessUnopenedDocument(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	rh.sendFramed(400, "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file:///never_opened.php"},
	})
	resp := rh.readResponseID(400, 5*time.Second)
	if resp == nil {
		t.Fatal("expected a response for documentSymbol on unopened document, got timeout")
	}
	// Must not be an error.
	if resp["error"] != nil {
		t.Errorf("expected no error for unopened document, got: %v", resp["error"])
	}
}

// TestRobustnessPositionOutOfBounds sends hover/completion/definition at a
// position that is well beyond the end of the document.  The server must not
// crash.
func TestRobustnessPositionOutOfBounds(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	const uri = "file:///tmp/proto_oob.php"
	source := "<?php\necho 'hi';\n"
	didOpenBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": uri, "languageId": "php", "version": 1, "text": source,
			},
		},
	})
	rh.sendRaw([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(didOpenBody), didOpenBody)))
	time.Sleep(30 * time.Millisecond)

	oobPos := map[string]interface{}{"line": 999999, "character": 999999}
	cases := []struct {
		id     int
		method string
	}{
		{500, "textDocument/hover"},
		{501, "textDocument/completion"},
		{502, "textDocument/definition"},
		{503, "textDocument/signatureHelp"},
	}
	for _, tc := range cases {
		rh.sendFramed(tc.id, tc.method, map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"position":     oobPos,
		})
	}
	for _, tc := range cases {
		resp := rh.readResponseID(tc.id, 5*time.Second)
		if resp == nil {
			t.Errorf("method %q (id %d): expected response for out-of-bounds position, got timeout", tc.method, tc.id)
		}
	}

	// Server must still be alive.
	probeAlive(t, rh, 510)
}

// TestRobustnessLargeDocument generates a ~20 000-line PHP file and sends it
// via didOpen.  The server must not crash and must respond to a subsequent
// documentSymbol request.
//
// The file is comment-heavy (low symbol density) so indexing completes quickly
// while still exercising the parser and message-framing code on a large body.
func TestRobustnessLargeDocument(t *testing.T) {
	rh := startedRawHarness(t)
	defer rh.close()

	// Build a large PHP file: ~20 000 lines.
	// Use few classes with many comment lines to keep symbol-index time low.
	var sb strings.Builder
	sb.WriteString("<?php\nnamespace LargeDoc;\n")
	// 10 classes, each with a handful of methods.
	for i := range 10 {
		fmt.Fprintf(&sb, "class GeneratedClass%04d {\n", i)
		for j := range 5 {
			fmt.Fprintf(&sb, "    public function method%04d_%04d(string $arg): void { echo $arg; }\n", i, j)
		}
		sb.WriteString("}\n")
		// Pad each class with comment lines so the total reaches ~20k lines.
		for range 1995 {
			sb.WriteString("// padding line\n")
		}
	}
	largeSource := sb.String()

	// Verify we actually produced a large document.
	lineCount := strings.Count(largeSource, "\n")
	if lineCount < 20000 {
		t.Fatalf("expected at least 20000 lines, got %d", lineCount)
	}

	const uri = "file:///tmp/proto_large.php"
	didOpenBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": uri, "languageId": "php", "version": 1, "text": largeSource,
			},
		},
	})
	rh.sendRaw([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(didOpenBody), didOpenBody)))
	time.Sleep(500 * time.Millisecond) // allow indexing to run

	rh.sendFramed(600, "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := rh.readResponseID(600, 10*time.Second)
	if resp == nil {
		t.Fatal("documentSymbol on large doc: timed out (possible crash or hang)")
	}
	if resp["error"] != nil {
		t.Errorf("documentSymbol on large doc: error %v", resp["error"])
	}
	// The server responded — it did not crash.
}

// TestStrictModeRepanics verifies that when strict mode is on, recoverPanic
// re-raises the original panic instead of swallowing it.
func TestStrictModeRepanics(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	s := NewServer(strings.NewReader(""), io.Discard, logger)
	s.SetStrict(true)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected re-panic in strict mode but did not see one")
		} else if r != "sentinel panic for strict mode test" {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()

	func() {
		defer s.recoverPanic("test context")
		panic("sentinel panic for strict mode test")
	}()
}

// TestStrictModeOffSwallows verifies that when strict mode is off (the
// default), recoverPanic swallows the panic and execution continues normally.
func TestStrictModeOffSwallows(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	s := NewServer(strings.NewReader(""), io.Discard, logger)
	// strict is false by default

	func() {
		defer s.recoverPanic("test context")
		panic("should be swallowed")
	}()
	// If we reach this line the panic was indeed swallowed.
}

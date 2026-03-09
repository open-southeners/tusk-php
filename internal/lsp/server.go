package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/open-southeners/php-lsp/internal/analyzer"
	"github.com/open-southeners/php-lsp/internal/completion"
	"github.com/open-southeners/php-lsp/internal/config"
	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/diagnostics"
	"github.com/open-southeners/php-lsp/internal/hover"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

const ServerName = "php-lsp"
const ServerVersion = "0.1.0"

type Server struct {
	cfg        *config.Config
	index      *symbols.Index
	container  *container.ContainerAnalyzer
	completion *completion.Provider
	hover      *hover.Provider
	diag       *diagnostics.Provider
	analyzer   *analyzer.Analyzer
	documents  sync.Map
	rootPath   string
	framework  string
	reader     *bufio.Reader
	writer     io.Writer
	logger     *log.Logger
	shutdown   bool
}

func NewServer(reader io.Reader, writer io.Writer, logger *log.Logger) *Server {
	return &Server{cfg: config.DefaultConfig(), index: symbols.NewIndex(), reader: bufio.NewReader(reader), writer: writer, logger: logger}
}

func (s *Server) Run() error {
	s.logger.Println("PHP LSP server starting...")
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF { return nil }
			s.logger.Printf("Read error: %v", err)
			continue
		}
		s.handleMessage(msg)
	}
}

type jsonRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) readMessage() (*jsonRPCMessage, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil { return nil, err }
		line = strings.TrimSpace(line)
		if line == "" { break }
		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil { return nil, fmt.Errorf("invalid Content-Length: %v", err) }
		}
	}
	if contentLength == 0 { return nil, fmt.Errorf("missing Content-Length") }
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil { return nil, err }
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil { return nil, err }
	s.logger.Printf("← %s", msg.Method)
	return &msg, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result interface{}) {
	if id == nil { return }
	s.writeMessage(jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	if id == nil { return }
	s.writeMessage(jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) sendNotification(method string, params interface{}) {
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) writeMessage(msg interface{}) {
	body, err := json.Marshal(msg)
	if err != nil { s.logger.Printf("Marshal error: %v", err); return }
	fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *Server) handleMessage(msg *jsonRPCMessage) {
	switch msg.Method {
	case "initialize": s.handleInitialize(msg)
	case "initialized": s.handleInitialized(msg)
	case "shutdown": s.shutdown = true; s.sendResponse(msg.ID, nil)
	case "exit": os.Exit(0)
	case "textDocument/didOpen": s.handleDidOpen(msg)
	case "textDocument/didChange": s.handleDidChange(msg)
	case "textDocument/didClose": s.handleDidClose(msg)
	case "textDocument/didSave": s.handleDidSave(msg)
	case "textDocument/completion": s.handleCompletion(msg)
	case "textDocument/hover": s.handleHover(msg)
	case "textDocument/definition": s.handleDefinition(msg)
	case "textDocument/references": s.handleReferences(msg)
	case "textDocument/documentSymbol": s.handleDocumentSymbol(msg)
	case "textDocument/signatureHelp": s.handleSignatureHelp(msg)
	default:
		if msg.ID != nil { s.sendError(msg.ID, -32601, fmt.Sprintf("Method not found: %s", msg.Method)) }
	}
}

func (s *Server) handleInitialize(msg *jsonRPCMessage) {
	var params protocol.InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	s.rootPath = strings.TrimPrefix(params.RootURI, "file://")
	if s.rootPath == "" { s.rootPath = params.RootPath }
	s.logger.Printf("Initializing for workspace: %s", s.rootPath)
	cfgPath := filepath.Join(s.rootPath, ".php-lsp.json")
	if cfg, err := config.LoadFromFile(cfgPath); err == nil { s.cfg = cfg }
	if s.cfg.Framework == "auto" { s.framework = config.DetectFramework(s.rootPath) } else { s.framework = s.cfg.Framework }
	s.logger.Printf("Detected framework: %s", s.framework)
	s.index.RegisterBuiltins()
	s.container = container.NewContainerAnalyzer(s.index, s.rootPath, s.framework)
	s.completion = completion.NewProvider(s.index, s.container, s.framework)
	s.hover = hover.NewProvider(s.index, s.container, s.framework)
	s.diag = diagnostics.NewProvider(s.index, s.framework)
	s.analyzer = analyzer.NewAnalyzer(s.index, s.container)
	s.sendResponse(msg.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: 1,
			CompletionProvider: &protocol.CompletionOptions{TriggerCharacters: []string{".", ">", ":", "$", "\\", "|", "#", "["}, ResolveProvider: false},
			HoverProvider: true, DefinitionProvider: true, ReferencesProvider: true, DocumentSymbolProvider: true,
			SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			DiagnosticProvider: &protocol.DiagnosticOptions{InterFileDependencies: true},
		},
		ServerInfo: protocol.ServerInfo{Name: ServerName, Version: ServerVersion},
	})
}

func (s *Server) handleInitialized(msg *jsonRPCMessage) {
	go s.indexWorkspace()
	go s.container.Analyze()
}

func (s *Server) handleDidOpen(msg *jsonRPCMessage) {
	var params protocol.DidOpenTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil { return }
	s.documents.Store(params.TextDocument.URI, params.TextDocument.Text)
	s.index.IndexFile(params.TextDocument.URI, params.TextDocument.Text)
	if s.cfg.DiagnosticsEnabled { s.publishDiagnostics(params.TextDocument.URI, params.TextDocument.Text) }
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	var params protocol.DidChangeTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil { return }
	if len(params.ContentChanges) > 0 {
		source := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.documents.Store(params.TextDocument.URI, source)
		s.index.IndexFile(params.TextDocument.URI, source)
		if s.cfg.DiagnosticsEnabled { s.publishDiagnostics(params.TextDocument.URI, source) }
	}
}

func (s *Server) handleDidClose(msg *jsonRPCMessage) {
	var params protocol.DidCloseTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil { return }
	s.documents.Delete(params.TextDocument.URI)
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	var params protocol.DidSaveTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil { return }
	if params.Text != nil {
		s.documents.Store(params.TextDocument.URI, *params.Text)
		s.index.IndexFile(params.TextDocument.URI, *params.Text)
	}
	go s.container.Analyze()
}

func (s *Server) handleCompletion(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.completion.GetCompletions(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.hover.GetHover(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindDefinition(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleReferences(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindReferences(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleDocumentSymbol(msg *jsonRPCMessage) {
	var params struct{ TextDocument protocol.TextDocumentIdentifier `json:"textDocument"` }
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentSymbols(params.TextDocument.URI, source))
}

func (s *Server) handleSignatureHelp(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil { s.sendError(msg.ID, -32602, "Invalid params"); return }
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetSignatureHelp(params.TextDocument.URI, source, params.Position))
}

func (s *Server) getDocument(uri string) string {
	if source, ok := s.documents.Load(uri); ok { return source.(string) }
	return ""
}

func (s *Server) publishDiagnostics(uri, source string) {
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{"uri": uri, "diagnostics": s.diag.Analyze(uri, source)})
}

func (s *Server) indexWorkspace() {
	s.logger.Printf("Indexing workspace: %s", s.rootPath)
	count := 0
	filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil { return nil }
		if info.IsDir() {
			for _, ex := range s.cfg.ExcludePaths { if filepath.Base(path) == ex { return filepath.SkipDir } }
			return nil
		}
		if !strings.HasSuffix(path, ".php") { return nil }
		if count >= s.cfg.MaxIndexFiles { return filepath.SkipAll }
		if content, err := os.ReadFile(path); err == nil { s.index.IndexFile("file://"+path, string(content)); count++ }
		return nil
	})
	s.logger.Printf("Indexed %d PHP files", count)
	s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d files (%s framework)", count, s.framework)})
}

package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/open-southeners/php-lsp/internal/analyzer"
	"github.com/open-southeners/php-lsp/internal/completion"
	"github.com/open-southeners/php-lsp/internal/composer"
	"github.com/open-southeners/php-lsp/internal/config"
	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/diagnostics"
	"github.com/open-southeners/php-lsp/internal/hover"
	"github.com/open-southeners/php-lsp/internal/models"
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
	docMu      sync.RWMutex
	documents  map[string]string
	rootPath   string
	framework  string
	reader     *bufio.Reader
	writer     io.Writer
	logger     *log.Logger
	shutdown   bool
}

func NewServer(reader io.Reader, writer io.Writer, logger *log.Logger) *Server {
	return &Server{cfg: config.DefaultConfig(), index: symbols.NewIndex(), documents: make(map[string]string), reader: bufio.NewReader(reader), writer: writer, logger: logger}
}

func (s *Server) Run() error {
	s.logger.Println("PHP LSP server starting...")
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			s.logger.Printf("Read error: %v", err)
			continue
		}
		func() {
			defer s.recoverPanic("handleMessage")
			s.handleMessage(msg)
		}()
	}
}

func (s *Server) recoverPanic(context string) {
	if r := recover(); r != nil {
		s.logger.Printf("Panic in %s: %v\n%s", context, r, debug.Stack())
	}
}

func (s *Server) goSafe(context string, fn func()) {
	go func() {
		defer s.recoverPanic(context)
		fn()
	}()
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
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %v", err)
			}
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return nil, err
	}
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	s.logger.Printf("← %s", msg.Method)
	return &msg, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result interface{}) {
	if id == nil {
		return
	}
	s.writeMessage(struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Result  interface{} `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	if id == nil {
		return
	}
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
	if err != nil {
		s.logger.Printf("Marshal error: %v", err)
		return
	}
	fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *Server) handleMessage(msg *jsonRPCMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(msg)
	case "shutdown":
		s.shutdown = true
		s.sendResponse(msg.ID, nil)
	case "exit":
		os.Exit(0)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/references":
		s.handleReferences(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(msg)
	case "textDocument/prepareRename":
		s.handlePrepareRename(msg)
	case "textDocument/rename":
		s.handleRename(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "workspace/executeCommand":
		s.handleExecuteCommand(msg)
	default:
		if msg.ID != nil {
			s.sendError(msg.ID, -32601, fmt.Sprintf("Method not found: %s", msg.Method))
		}
	}
}

func (s *Server) handleInitialize(msg *jsonRPCMessage) {
	var params protocol.InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	s.rootPath = strings.TrimPrefix(params.RootURI, "file://")
	if s.rootPath == "" {
		s.rootPath = params.RootPath
	}
	s.logger.Printf("Initializing for workspace: %s", s.rootPath)
	cfgPath := filepath.Join(s.rootPath, ".php-lsp.json")
	if cfg, err := config.LoadFromFile(cfgPath); err == nil {
		s.cfg = cfg
	}
	// Merge client initializationOptions over file-based config
	if opts := params.InitializationOptions; opts != nil {
		s.cfg.MergeClientOptions(opts)
	}
	if s.cfg.Framework == "auto" {
		s.framework = config.DetectFramework(s.rootPath)
	} else {
		s.framework = s.cfg.Framework
	}
	s.logger.Printf("Detected framework: %s", s.framework)
	s.index.RegisterBuiltins()
	s.container = container.NewContainerAnalyzer(s.index, s.rootPath, s.framework)
	arrayResolver := models.NewFrameworkArrayResolver(s.index, s.rootPath, s.framework)
	s.completion = completion.NewProvider(s.index, s.container, s.framework)
	s.completion.SetArrayResolver(arrayResolver)
	s.hover = hover.NewProvider(s.index, s.container, s.framework)
	s.hover.SetArrayResolver(arrayResolver)
	s.diag = diagnostics.NewProvider(s.index, s.framework, s.rootPath, s.logger, s.cfg)
	s.analyzer = analyzer.NewAnalyzer(s.index, s.container)
	s.sendResponse(msg.ID, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full
				Save:      &protocol.SaveOptions{IncludeText: false},
			},
			CompletionProvider: &protocol.CompletionOptions{TriggerCharacters: []string{".", ">", ":", "$", "\\", "|", "#", "[", "(", "'", "\""}, ResolveProvider: false},
			HoverProvider:      true, DefinitionProvider: true, ReferencesProvider: true, DocumentSymbolProvider: true,
			SignatureHelpProvider: &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			RenameProvider:       &protocol.RenameOptions{PrepareProvider: true},
			CodeActionProvider:   &protocol.CodeActionOptions{CodeActionKinds: []string{"refactor", "source"}},
			ExecuteCommandProvider: &protocol.ExecuteCommandOptions{Commands: []string{"phpLsp.namespaceForPath"}},
		},
		ServerInfo: protocol.ServerInfo{Name: ServerName, Version: ServerVersion},
	})
}

func (s *Server) handleInitialized(msg *jsonRPCMessage) {
	s.goSafe("indexWorkspace", s.indexWorkspace)
	s.goSafe("indexComposerDeps", s.indexComposerDependencies)
	s.goSafe("container.Analyze", s.container.Analyze)
}

func (s *Server) handleDidOpen(msg *jsonRPCMessage) {
	var params protocol.DidOpenTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	s.docMu.Lock()
	s.documents[params.TextDocument.URI] = params.TextDocument.Text
	s.docMu.Unlock()
	s.index.IndexFile(params.TextDocument.URI, params.TextDocument.Text)
	if s.cfg.DiagnosticsEnabled {
		s.publishDiagnostics(params.TextDocument.URI, params.TextDocument.Text)
	}
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	var params protocol.DidChangeTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	if len(params.ContentChanges) > 0 {
		source := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.docMu.Lock()
		s.documents[params.TextDocument.URI] = source
		s.docMu.Unlock()
		s.index.IndexFile(params.TextDocument.URI, source)
		if s.cfg.DiagnosticsEnabled {
			s.publishDiagnostics(params.TextDocument.URI, source)
		}
	}
}

func (s *Server) handleDidClose(msg *jsonRPCMessage) {
	var params protocol.DidCloseTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	s.docMu.Lock()
	delete(s.documents, params.TextDocument.URI)
	s.docMu.Unlock()
	s.diag.ClearCache(params.TextDocument.URI)
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	var params protocol.DidSaveTextDocumentParams
	if json.Unmarshal(msg.Params, &params) != nil {
		return
	}
	uri := params.TextDocument.URI
	if params.Text != nil {
		s.docMu.Lock()
		s.documents[uri] = *params.Text
		s.docMu.Unlock()
		s.index.IndexFile(uri, *params.Text)
	}
	s.goSafe("container.Analyze", s.container.Analyze)
	if s.cfg.DiagnosticsEnabled {
		s.goSafe("diagnostics.RunTools", func() {
			filePath := strings.TrimPrefix(uri, "file://")
			s.diag.RunTools(uri, filePath)
			source := s.getDocument(uri)
			s.publishDiagnostics(uri, source)
		})
	}
}

func (s *Server) handleCompletion(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.completion.GetCompletions(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.hover.GetHover(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindDefinition(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleReferences(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.FindReferences(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handleDocumentSymbol(msg *jsonRPCMessage) {
	var params struct {
		TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetDocumentSymbols(params.TextDocument.URI, source))
}

func (s *Server) handleSignatureHelp(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	s.sendResponse(msg.ID, s.analyzer.GetSignatureHelp(params.TextDocument.URI, source, params.Position))
}

func (s *Server) handlePrepareRename(msg *jsonRPCMessage) {
	var params protocol.TextDocumentPositionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.PrepareRename(params.TextDocument.URI, source, params.Position)
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleRename(msg *jsonRPCMessage) {
	var params protocol.RenameParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.Rename(params.TextDocument.URI, source, params.Position, params.NewName, s.getDocumentReader())
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleCodeAction(msg *jsonRPCMessage) {
	var params protocol.CodeActionParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	source := s.getDocument(params.TextDocument.URI)
	result := s.analyzer.GetCodeActions(params.TextDocument.URI, source, params)
	s.sendResponse(msg.ID, result)
}

func (s *Server) handleExecuteCommand(msg *jsonRPCMessage) {
	var params protocol.ExecuteCommandParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	switch params.Command {
	case "phpLsp.copyNamespace":
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				source := s.getDocument(uri)
				ns := s.analyzer.GetFileNamespace(uri, source)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "phpLsp.namespaceForPath":
		// Returns the expected namespace for a file path based on PSR-4 autoload
		if len(params.Arguments) > 0 {
			var uri string
			if json.Unmarshal(params.Arguments[0], &uri) == nil {
				filePath := strings.TrimPrefix(uri, "file://")
				autoload := composer.GetAutoloadPaths(s.rootPath)
				ns := composer.PathToNamespace(filePath, autoload)
				s.sendResponse(msg.ID, ns)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	case "phpLsp.moveToNamespace":
		// Arguments: [uri, targetNamespace]
		if len(params.Arguments) >= 2 {
			var uri, targetNS string
			if json.Unmarshal(params.Arguments[0], &uri) == nil && json.Unmarshal(params.Arguments[1], &targetNS) == nil {
				source := s.getDocument(uri)
				autoload := composer.GetAutoloadPaths(s.rootPath)
				edit := s.analyzer.MoveToNamespace(uri, source, targetNS, autoload, s.getDocumentReader())
				s.sendResponse(msg.ID, edit)
				return
			}
		}
		s.sendResponse(msg.ID, nil)
	default:
		s.sendError(msg.ID, -32601, fmt.Sprintf("Unknown command: %s", params.Command))
	}
}

// getDocumentReader returns a function that reads document content by URI,
// falling back to disk if the document isn't open in the editor.
func (s *Server) getDocumentReader() func(string) string {
	return func(uri string) string {
		s.docMu.RLock()
		source, ok := s.documents[uri]
		s.docMu.RUnlock()
		if ok {
			return source
		}
		// Fall back to reading from disk
		path := strings.TrimPrefix(uri, "file://")
		content, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(content)
	}
}

func (s *Server) getDocument(uri string) string {
	s.docMu.RLock()
	source := s.documents[uri]
	s.docMu.RUnlock()
	return source
}

func (s *Server) publishDiagnostics(uri, source string) {
	diagnostics := s.diag.Analyze(uri, source)
	if diagnostics == nil {
		diagnostics = []protocol.Diagnostic{}
	}
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{"uri": uri, "diagnostics": diagnostics})
}

func (s *Server) indexWorkspace() {
	s.logger.Printf("Indexing workspace: %s", s.rootPath)
	count := 0
	filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			for _, ex := range s.cfg.ExcludePaths {
				if filepath.Base(path) == ex {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".php") {
			return nil
		}
		if count >= s.cfg.MaxIndexFiles {
			return filepath.SkipAll
		}
		if content, err := os.ReadFile(path); err == nil {
			func() {
				defer s.recoverPanic("indexWorkspace:" + path)
				s.index.IndexFileWithSource("file://"+path, string(content), symbols.SourceProject)
				count++
			}()
		}
		return nil
	})
	s.logger.Printf("Indexed %d PHP files", count)
	s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d files (%s framework)", count, s.framework)})
}

func (s *Server) indexComposerDependencies() {
	entries := composer.GetAutoloadPaths(s.rootPath)
	if len(entries) == 0 {
		return
	}
	vendorCount := 0
	for _, entry := range entries {
		src := symbols.SourceProject
		if entry.IsVendor {
			src = symbols.SourceVendor
		}

		if entry.IsFile {
			if content, err := os.ReadFile(entry.Path); err == nil {
				func() {
					defer s.recoverPanic("indexFile:" + entry.Path)
					s.index.IndexFileWithSource("file://"+entry.Path, string(content), src)
					if entry.IsVendor {
						vendorCount++
					}
				}()
			}
			continue
		}

		if !entry.IsVendor {
			continue
		}
		info, err := os.Stat(entry.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		filepath.Walk(entry.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".php") {
				return nil
			}
			if content, err := os.ReadFile(path); err == nil {
				func() {
					defer s.recoverPanic("indexVendor:" + path)
					s.index.IndexFileWithSource("file://"+path, string(content), symbols.SourceVendor)
					vendorCount++
				}()
			}
			return nil
		})
	}
	s.logger.Printf("Indexed %d vendor PHP files from Composer dependencies", vendorCount)
	if vendorCount > 0 {
		s.sendNotification("window/logMessage", map[string]interface{}{"type": protocol.MessageTypeInfo, "message": fmt.Sprintf("PHP LSP: Indexed %d vendor files", vendorCount)})
	}
}

package protocol

import "encoding/json"

// MessageType represents LSP message types.
type MessageType int

const (
	MessageTypeError   MessageType = 1
	MessageTypeWarning MessageType = 2
	MessageTypeInfo    MessageType = 3
	MessageTypeLog     MessageType = 4
)

// CompletionItemKind as defined by LSP spec.
type CompletionItemKind int

const (
	CompletionItemKindText          CompletionItemKind = 1
	CompletionItemKindMethod        CompletionItemKind = 2
	CompletionItemKindFunction      CompletionItemKind = 3
	CompletionItemKindConstructor   CompletionItemKind = 4
	CompletionItemKindField         CompletionItemKind = 5
	CompletionItemKindVariable      CompletionItemKind = 6
	CompletionItemKindClass         CompletionItemKind = 7
	CompletionItemKindInterface     CompletionItemKind = 8
	CompletionItemKindModule        CompletionItemKind = 9
	CompletionItemKindProperty      CompletionItemKind = 10
	CompletionItemKindUnit          CompletionItemKind = 11
	CompletionItemKindValue         CompletionItemKind = 12
	CompletionItemKindEnum          CompletionItemKind = 13
	CompletionItemKindKeyword       CompletionItemKind = 14
	CompletionItemKindSnippet       CompletionItemKind = 15
	CompletionItemKindConstant      CompletionItemKind = 16
	CompletionItemKindEnumMember    CompletionItemKind = 17
	CompletionItemKindStruct        CompletionItemKind = 18
	CompletionItemKindEvent         CompletionItemKind = 19
	CompletionItemKindOperator      CompletionItemKind = 20
	CompletionItemKindTypeParameter CompletionItemKind = 21
)

// DiagnosticSeverity as defined by LSP spec.
type DiagnosticSeverity int

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

// SymbolKind as defined by LSP spec.
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindTypeParameter SymbolKind = 26
)

// Position in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location inside a resource.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic represents a diagnostic (error, warning, etc.).
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Code     string             `json:"code,omitempty"`
}

// CompletionItem represents a completion suggestion.
type CompletionItem struct {
	Label               string             `json:"label"`
	Kind                CompletionItemKind `json:"kind"`
	Detail              string             `json:"detail,omitempty"`
	Documentation       string             `json:"documentation,omitempty"`
	InsertText          string             `json:"insertText,omitempty"`
	InsertTextFormat    int                `json:"insertTextFormat,omitempty"`
	SortText            string             `json:"sortText,omitempty"`
	FilterText          string             `json:"filterText,omitempty"`
	Deprecated          bool               `json:"deprecated,omitempty"`
	AdditionalTextEdits []TextEdit         `json:"additionalTextEdits,omitempty"`
}

// Hover represents the result of a hover request.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent represents a string value with a specific kind.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DocumentSymbol represents a symbol found in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// TextDocumentIdentifier identifies a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem is an item to transfer a text document from client to server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams is a parameter for position-based requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// TextDocumentContentChangeEvent describes a content change event.
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// DidOpenTextDocumentParams is for textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams is for textDocument/didChange.
type DidChangeTextDocumentParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidCloseTextDocumentParams is for textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams is for textDocument/didSave.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// InitializeParams for the initialize request.
type InitializeParams struct {
	ProcessID             *int                    `json:"processId"`
	RootURI               string                  `json:"rootUri"`
	RootPath              string                  `json:"rootPath"`
	InitializationOptions *InitializationOptions  `json:"initializationOptions,omitempty"`
	Capabilities          struct {
		TextDocument struct {
			Completion struct {
				CompletionItem struct {
					SnippetSupport bool `json:"snippetSupport"`
				} `json:"completionItem"`
			} `json:"completion"`
		} `json:"textDocument"`
	} `json:"capabilities"`
}

// InitializationOptions sent by the client during initialization.
type InitializationOptions struct {
	PHPVersion         string   `json:"phpVersion,omitempty"`
	Framework          string   `json:"framework,omitempty"`
	ContainerAware     *bool    `json:"containerAware,omitempty"`
	DiagnosticsEnabled *bool    `json:"diagnosticsEnabled,omitempty"`
	PHPStanEnabled     *bool    `json:"phpstanEnabled,omitempty"`
	PHPStanPath        string   `json:"phpstanPath,omitempty"`
	PHPStanLevel       string   `json:"phpstanLevel,omitempty"`
	PHPStanConfig      string   `json:"phpstanConfig,omitempty"`
	PintEnabled        *bool    `json:"pintEnabled,omitempty"`
	PintPath           string   `json:"pintPath,omitempty"`
	PintConfig         string   `json:"pintConfig,omitempty"`
	DatabaseEnabled    *bool    `json:"databaseEnabled,omitempty"`
	MaxIndexFiles      *int     `json:"maxIndexFiles,omitempty"`
	ExcludePaths       []string `json:"excludePaths,omitempty"`
}

// InitializeResult for the initialize response.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

// ServerInfo contains information about the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// TextDocumentSyncOptions describes how text document syncing works.
type TextDocumentSyncOptions struct {
	OpenClose bool              `json:"openClose"`
	Change    int               `json:"change"` // 0=None, 1=Full, 2=Incremental
	Save      *SaveOptions     `json:"save,omitempty"`
}

// SaveOptions for textDocument/didSave notifications.
type SaveOptions struct {
	IncludeText bool `json:"includeText"`
}

// ServerCapabilities declares server capabilities.
type ServerCapabilities struct {
	TextDocumentSync           TextDocumentSyncOptions     `json:"textDocumentSync"`
	CompletionProvider         *CompletionOptions          `json:"completionProvider,omitempty"`
	HoverProvider              bool                        `json:"hoverProvider"`
	DefinitionProvider         bool                        `json:"definitionProvider"`
	ReferencesProvider         bool                        `json:"referencesProvider"`
	DocumentSymbolProvider     bool                        `json:"documentSymbolProvider"`
	WorkspaceSymbolProvider    bool                        `json:"workspaceSymbolProvider"`
	DiagnosticProvider         *DiagnosticOptions          `json:"diagnosticProvider,omitempty"`
	SignatureHelpProvider      *SignatureHelpOptions        `json:"signatureHelpProvider,omitempty"`
	DocumentFormattingProvider bool                        `json:"documentFormattingProvider"`
	RenameProvider             *RenameOptions              `json:"renameProvider,omitempty"`
	CodeActionProvider         *CodeActionOptions          `json:"codeActionProvider,omitempty"`
	ExecuteCommandProvider     *ExecuteCommandOptions      `json:"executeCommandProvider,omitempty"`
}

// CompletionOptions for completion requests.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider"`
}

// SignatureHelpOptions for signature help requests.
type SignatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// DiagnosticOptions for diagnostics.
type DiagnosticOptions struct {
	InterFileDependencies bool `json:"interFileDependencies"`
	WorkspaceDiagnostics  bool `json:"workspaceDiagnostics"`
}

// SignatureHelp represents the signature of a callable.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// SignatureInformation represents a callable signature.
type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation string                 `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

// ParameterInformation represents a parameter of a callable.
type ParameterInformation struct {
	Label         string `json:"label"`
	Documentation string `json:"documentation,omitempty"`
}

// --- Rename types ---

// RenameOptions for rename support.
type RenameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

// RenameParams for textDocument/rename.
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// PrepareRenameResult is the response for textDocument/prepareRename.
type PrepareRenameResult struct {
	Range       Range  `json:"range"`
	Placeholder string `json:"placeholder"`
}

// --- Workspace edit types ---

// TextEdit is a textual edit applicable to a text document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit represents changes to many resources managed in the workspace.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []DocumentChange      `json:"documentChanges,omitempty"`
}

// DocumentChange is a union type that can be a TextDocumentEdit or a file operation.
// Only one field should be set.
type DocumentChange struct {
	TextDocumentEdit *TextDocumentEdit `json:"-"`
	RenameFile       *RenameFile       `json:"-"`
	CreateFile       *CreateFile       `json:"-"`
	DeleteFile       *DeleteFile       `json:"-"`
}

// MarshalJSON implements custom marshaling for the DocumentChange union type.
func (dc DocumentChange) MarshalJSON() ([]byte, error) {
	if dc.TextDocumentEdit != nil {
		return json.Marshal(dc.TextDocumentEdit)
	}
	if dc.RenameFile != nil {
		return json.Marshal(dc.RenameFile)
	}
	if dc.CreateFile != nil {
		return json.Marshal(dc.CreateFile)
	}
	if dc.DeleteFile != nil {
		return json.Marshal(dc.DeleteFile)
	}
	return []byte("null"), nil
}

// TextDocumentEdit describes textual changes on a single text document.
type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// VersionedTextDocumentIdentifier identifies a specific version of a text document.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version"` // null means the version is unknown
}

// RenameFile is a file operation to rename/move a file.
type RenameFile struct {
	Kind   string             `json:"kind"` // always "rename"
	OldURI string             `json:"oldUri"`
	NewURI string             `json:"newUri"`
	Options *RenameFileOptions `json:"options,omitempty"`
}

// RenameFileOptions for rename file operations.
type RenameFileOptions struct {
	Overwrite      bool `json:"overwrite,omitempty"`
	IgnoreIfExists bool `json:"ignoreIfExists,omitempty"`
}

// CreateFile is a file operation to create a file.
type CreateFile struct {
	Kind    string              `json:"kind"` // always "create"
	URI     string              `json:"uri"`
	Options *CreateFileOptions  `json:"options,omitempty"`
}

// CreateFileOptions for create file operations.
type CreateFileOptions struct {
	Overwrite      bool `json:"overwrite,omitempty"`
	IgnoreIfExists bool `json:"ignoreIfExists,omitempty"`
}

// DeleteFile is a file operation to delete a file.
type DeleteFile struct {
	Kind    string              `json:"kind"` // always "delete"
	URI     string              `json:"uri"`
	Options *DeleteFileOptions  `json:"options,omitempty"`
}

// DeleteFileOptions for delete file operations.
type DeleteFileOptions struct {
	Recursive         bool `json:"recursive,omitempty"`
	IgnoreIfNotExists bool `json:"ignoreIfNotExists,omitempty"`
}

// --- Code action types ---

// CodeActionOptions for code action support.
type CodeActionOptions struct {
	CodeActionKinds []string `json:"codeActionKinds,omitempty"`
}

// CodeActionParams for textDocument/codeAction.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext carries additional information about the context in which
// a code action is requested.
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

// CodeAction represents a change that can be performed in code.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
}

// Command represents a reference to a command.
type Command struct {
	Title     string            `json:"title"`
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments,omitempty"`
}

// --- Execute command types ---

// ExecuteCommandOptions for executeCommand support.
type ExecuteCommandOptions struct {
	Commands []string `json:"commands"`
}

// ExecuteCommandParams for workspace/executeCommand.
type ExecuteCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments,omitempty"`
}

// --- Server-to-client request types ---

// ApplyWorkspaceEditParams for workspace/applyEdit (server → client).
type ApplyWorkspaceEditParams struct {
	Label string        `json:"label,omitempty"`
	Edit  WorkspaceEdit `json:"edit"`
}

// ApplyWorkspaceEditResult is the client's response to workspace/applyEdit.
type ApplyWorkspaceEditResult struct {
	Applied bool `json:"applied"`
}

package diagnostics

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

type phpstanRunner struct {
	binPath  string
	level    string
	cfgFile  string
	rootPath string
	logger   *log.Logger
	enabled  bool
}

type phpstanOutput struct {
	Totals struct {
		Errors     int `json:"errors"`
		FileErrors int `json:"file_errors"`
	} `json:"totals"`
	Files map[string]phpstanFileResult `json:"files"`
}

type phpstanFileResult struct {
	Errors   int              `json:"errors"`
	Messages []phpstanMessage `json:"messages"`
}

type phpstanMessage struct {
	Message    string `json:"message"`
	Line       int    `json:"line"`
	Ignorable  bool   `json:"ignorable"`
	Identifier string `json:"identifier"`
}

func newPHPStanRunner(rootPath string, cfg *config.Config, logger *log.Logger) *phpstanRunner {
	r := &phpstanRunner{
		rootPath: rootPath,
		logger:   logger,
		level:    cfg.PHPStanLevel,
	}

	if cfg.PHPStanEnabled != nil {
		r.enabled = *cfg.PHPStanEnabled
	} else {
		r.enabled = true
	}

	if cfg.PHPStanPath != "" {
		r.binPath = cfg.PHPStanPath
	} else {
		vendorBin := filepath.Join(rootPath, "vendor", "bin", "phpstan")
		if _, err := os.Stat(vendorBin); err == nil {
			r.binPath = vendorBin
		} else if path, err := exec.LookPath("phpstan"); err == nil {
			r.binPath = path
		} else {
			r.enabled = false
		}
	}

	if cfg.PHPStanConfig != "" {
		r.cfgFile = cfg.PHPStanConfig
	} else {
		for _, name := range []string{"phpstan.neon", "phpstan.neon.dist", "phpstan.dist.neon"} {
			candidate := filepath.Join(rootPath, name)
			if _, err := os.Stat(candidate); err == nil {
				r.cfgFile = candidate
				break
			}
		}
	}

	if r.enabled && r.binPath != "" {
		logger.Printf("PHPStan: binary=%s config=%s level=%s", r.binPath, r.cfgFile, r.level)
	}

	return r
}

func (r *phpstanRunner) available() bool {
	return r.enabled && r.binPath != ""
}

func (r *phpstanRunner) analyze(filePath string) []protocol.Diagnostic {
	args := []string{"analyse", "--error-format=json", "--no-progress", "--no-ansi"}

	if r.cfgFile != "" {
		args = append(args, "-c", r.cfgFile)
	}

	if r.level != "" {
		args = append(args, "--level="+r.level)
	}

	args = append(args, filePath)

	cmd := exec.Command(r.binPath, args...)
	cmd.Dir = r.rootPath

	// Output() returns stdout; on non-zero exit, err is *ExitError
	// but stdout still contains the JSON output.
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				// Exit code 1 = errors found (expected), anything else is a real error
				r.logger.Printf("PHPStan error (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
				return nil
			}
			// Exit code 1: errors found, stdout has JSON — fall through to parse
		} else {
			r.logger.Printf("PHPStan exec error: %v", err)
			return nil
		}
	}

	// Read source lines so we can compute accurate diagnostic ranges
	var lines []string
	if src, err := os.ReadFile(filePath); err == nil {
		lines = strings.Split(string(src), "\n")
	}

	return r.parseOutput(output, lines)
}

func (r *phpstanRunner) parseOutput(data []byte, sourceLines []string) []protocol.Diagnostic {
	jsonData := extractJSON(data)
	if jsonData == nil {
		return nil
	}

	var result phpstanOutput
	if err := json.Unmarshal(jsonData, &result); err != nil {
		r.logger.Printf("PHPStan JSON parse error: %v", err)
		return nil
	}

	var diags []protocol.Diagnostic
	for _, fileResult := range result.Files {
		for _, msg := range fileResult.Messages {
			line := msg.Line - 1 // PHPStan 1-based → LSP 0-based
			if line < 0 {
				line = 0
			}

			code := msg.Identifier
			if code == "" {
				code = "phpstan"
			}

			startCol, endCol := diagnosticRange(sourceLines, line, msg.Message)

			diags = append(diags, protocol.Diagnostic{
				Range: protocol.Range{
					Start: protocol.Position{Line: line, Character: startCol},
					End:   protocol.Position{Line: line, Character: endCol},
				},
				Severity: protocol.DiagnosticSeverityError,
				Source:   "phpstan",
				Message:  msg.Message,
				Code:     code,
			})
		}
	}

	return diags
}

// diagnosticRange computes the start and end columns for a diagnostic on a line.
// It tries to find the specific identifier mentioned in the PHPStan message,
// falling back to highlighting the trimmed line content.
func diagnosticRange(lines []string, line int, message string) (int, int) {
	if line < 0 || line >= len(lines) {
		return 0, 0
	}
	src := lines[line]

	// Try to extract a symbol name from common PHPStan message patterns and
	// highlight just that symbol on the line.
	candidates := extractMessageIdentifiers(message)
	for _, candidate := range candidates {
		if col := strings.Index(src, candidate); col >= 0 {
			return col, col + len(candidate)
		}
	}

	// Fallback: highlight the trimmed content of the line
	startCol := len(src) - len(strings.TrimLeft(src, " \t"))
	endCol := len(strings.TrimRight(src, " \t\r\n"))
	if endCol <= startCol {
		return 0, len(src)
	}
	return startCol, endCol
}

// extractMessageIdentifiers pulls identifiable symbols from PHPStan messages.
// Common patterns: "Function foo not found", "Call to method bar() on ...",
// "Access to property $baz on ...", "Class Foo\Bar not found", etc.
func extractMessageIdentifiers(msg string) []string {
	var candidates []string
	patterns := []struct {
		prefix string
		suffix string
	}{
		{"Function ", " not found"},
		{"Call to method ", "("},
		{"Call to static method ", "("},
		{"Call to undefined method ", "("},
		{"Access to an undefined property ", "."},
		{"Access to property ", " on"},
		{"Class ", " not found"},
		{"Instantiation of class ", " "},
		{"Property ", " ("},
		{"Variable $", " "},
		{"Constant ", " not found"},
		{"Method ", " "},
		{"Parameter $", " "},
		{"Parameter #", " "},
	}
	for _, p := range patterns {
		idx := strings.Index(msg, p.prefix)
		if idx < 0 {
			continue
		}
		rest := msg[idx+len(p.prefix):]
		if p.suffix != "" {
			if end := strings.Index(rest, p.suffix); end > 0 {
				name := rest[:end]
				// For methods like "Foo::bar", extract just "bar"
				if sepIdx := strings.LastIndex(name, "::"); sepIdx >= 0 {
					candidates = append(candidates, name[sepIdx+2:])
				}
				// For properties, keep $prefix
				if strings.HasPrefix(p.prefix, "Variable ") || strings.HasPrefix(p.prefix, "Parameter $") {
					name = "$" + name
				}
				candidates = append(candidates, name)
			}
		}
	}
	return candidates
}

// extractJSON finds the first JSON object in data, skipping any preamble.
func extractJSON(data []byte) []byte {
	s := strings.TrimSpace(string(data))
	start := strings.Index(s, "{")
	if start < 0 {
		return nil
	}
	return []byte(s[start:])
}

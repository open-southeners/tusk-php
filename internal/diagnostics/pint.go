package diagnostics

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

type pintRunner struct {
	binPath  string
	cfgFile  string
	rootPath string
	logger   *log.Logger
	enabled  bool
}

type pintOutput struct {
	Files []pintFile `json:"files"`
}

type pintFile struct {
	Name          string   `json:"name"`
	Diff          string   `json:"diff"`
	AppliedFixers []string `json:"appliedFixers"`
}

func newPintRunner(rootPath string, cfg *config.Config, logger *log.Logger) *pintRunner {
	r := &pintRunner{
		rootPath: rootPath,
		logger:   logger,
	}

	if cfg.PintEnabled != nil {
		r.enabled = *cfg.PintEnabled
	} else {
		r.enabled = true
	}

	if cfg.PintPath != "" {
		r.binPath = cfg.PintPath
	} else {
		vendorBin := filepath.Join(rootPath, "vendor", "bin", "pint")
		if _, err := os.Stat(vendorBin); err == nil {
			r.binPath = vendorBin
		} else if path, err := exec.LookPath("pint"); err == nil {
			r.binPath = path
		} else {
			r.enabled = false
		}
	}

	if cfg.PintConfig != "" {
		r.cfgFile = cfg.PintConfig
	} else {
		candidate := filepath.Join(rootPath, "pint.json")
		if _, err := os.Stat(candidate); err == nil {
			r.cfgFile = candidate
		}
	}

	if r.enabled && r.binPath != "" {
		logger.Printf("Pint: binary=%s config=%s", r.binPath, r.cfgFile)
	}

	return r
}

func (r *pintRunner) available() bool {
	return r.enabled && r.binPath != ""
}

func (r *pintRunner) analyze(filePath string) []protocol.Diagnostic {
	args := []string{"--test", "--format=json"}

	if r.cfgFile != "" {
		args = append(args, "--config="+r.cfgFile)
	}

	args = append(args, filePath)

	cmd := exec.Command(r.binPath, args...)
	cmd.Dir = r.rootPath

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// Exit code 1 = fixable issues found — stdout has JSON
			} else {
				r.logger.Printf("Pint error (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
				return nil
			}
		} else {
			r.logger.Printf("Pint exec error: %v", err)
			return nil
		}
	}

	return r.parseOutput(output)
}

func (r *pintRunner) parseOutput(data []byte) []protocol.Diagnostic {
	jsonData := extractJSON(data)
	if jsonData == nil {
		return nil
	}

	var result pintOutput
	if err := json.Unmarshal(jsonData, &result); err != nil {
		r.logger.Printf("Pint JSON parse error: %v", err)
		return nil
	}

	var diags []protocol.Diagnostic
	for _, file := range result.Files {
		if file.Diff != "" {
			diags = append(diags, parseDiffDiagnostics(file.Diff, file.AppliedFixers)...)
		} else {
			// No diff available — report each fixer at file level
			for _, fixer := range file.AppliedFixers {
				diags = append(diags, protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: 0, Character: 0},
						End:   protocol.Position{Line: 0, Character: 0},
					},
					Severity: protocol.DiagnosticSeverityWarning,
					Source:   "pint",
					Message:  formatFixerName(fixer),
					Code:     fixer,
				})
			}
		}
	}

	return diags
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@`)

// parseDiffDiagnostics extracts line-precise diagnostics from a unified diff.
// Removed lines (starting with '-') represent the original code that needs fixing.
func parseDiffDiagnostics(diff string, fixers []string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	fixerDesc := strings.Join(fixers, ", ")
	message := formatFixerName(fixerDesc)

	lines := strings.Split(diff, "\n")
	origLine := 0
	reported := make(map[int]bool)

	for _, line := range lines {
		// Skip diff file headers
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			origLine, _ = strconv.Atoi(m[1])
			continue
		}

		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case '-':
			// This original line needs changes — report it
			lspLine := origLine - 1 // 1-based → 0-based
			if lspLine < 0 {
				lspLine = 0
			}
			if !reported[lspLine] {
				content := strings.TrimSpace(line[1:])
				col := 0
				if content != "" {
					// Find column of first non-whitespace in the removed line
					trimmed := strings.TrimLeft(line[1:], " \t")
					col = len(line[1:]) - len(trimmed)
				}
				diags = append(diags, protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: lspLine, Character: col},
						End:   protocol.Position{Line: lspLine, Character: col + len(strings.TrimSpace(content))},
					},
					Severity: protocol.DiagnosticSeverityWarning,
					Source:   "pint",
					Message:  message,
					Code:     fixerDesc,
				})
				reported[lspLine] = true
			}
			origLine++
		case '+':
			// Added line — doesn't exist in original, don't increment
		default:
			// Context line
			origLine++
		}
	}

	return diags
}

func formatFixerName(name string) string {
	return "Code style: " + strings.ReplaceAll(name, "_", " ")
}

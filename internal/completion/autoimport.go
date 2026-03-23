package completion

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

// buildAutoImportEdit returns a TextEdit that adds a `use` statement for the given FQN
// if it is not already imported. Returns nil if no import is needed.
func buildAutoImportEdit(fqn string, source string, file *parser.FileNode) []protocol.TextEdit {
	if fqn == "" || file == nil {
		return nil
	}
	// No import needed for unnamespaced classes
	if !strings.Contains(fqn, "\\") {
		return nil
	}
	// Already in the same namespace
	if file.Namespace != "" {
		nsPrefix := file.Namespace + "\\"
		if strings.HasPrefix(fqn, nsPrefix) && !strings.Contains(fqn[len(nsPrefix):], "\\") {
			return nil
		}
	}
	// Already imported
	for _, u := range file.Uses {
		if u.FullName == fqn {
			return nil
		}
	}

	// Determine insertion line
	insertLine := 0
	if len(file.Uses) > 0 {
		// Insert after the last use statement
		lastUse := file.Uses[len(file.Uses)-1]
		insertLine = lastUse.StartLine + 1
	} else {
		// Insert after namespace declaration (with a blank line)
		lines := strings.Split(source, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "namespace ") {
				insertLine = i + 2 // blank line after namespace
				break
			}
		}
		if insertLine == 0 {
			// No namespace — insert after <?php
			insertLine = 1
		}
	}

	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: insertLine, Character: 0},
			End:   protocol.Position{Line: insertLine, Character: 0},
		},
		NewText: "use " + fqn + ";\n",
	}}
}

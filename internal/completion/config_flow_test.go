package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestConfigResultArrayKeyAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// config('database')['connections'] — cursor after ['
	source := `<?php
$db = config('database');
$db['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 5})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["default"] {
		t.Error("expected 'default' from config('database') array access")
	}
	if !labels["connections"] {
		t.Error("expected 'connections' from config('database') array access")
	}
}

func TestConfigResultNestedArrayKeyAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// $db = config('database'); $db['connections']['
	source := `<?php
$db = config('database');
$db['connections']['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 20})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' from $db['connections']['")
	}
	if !labels["sqlite"] {
		t.Error("expected 'sqlite' from $db['connections']['")
	}
}

func TestConfigDotNotationVariableAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// $conns = config('database.connections'); $conns['
	source := `<?php
$conns = config('database.connections');
$conns['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 8})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' from config('database.connections') variable access")
	}
	if !labels["sqlite"] {
		t.Error("expected 'sqlite' from config('database.connections') variable access")
	}
}

func TestConfigDeepNestedVariableAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// $db = config('database'); $db['connections']['sqlite']['
	source := `<?php
$db = config('database');
$db['connections']['sqlite']['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 30})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["driver"] {
		t.Error("expected 'driver' from deep nested config access")
	}
	if !labels["database"] {
		t.Error("expected 'database' from deep nested config access")
	}
}

func TestConfigResultDirectBracketAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// config('database')['connections'][' — direct bracket access on config() result
	source := `<?php
config('database')['connections']['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 35})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' from config('database')['connections']['")
	}
	if !labels["sqlite"] {
		t.Error("expected 'sqlite' from config('database')['connections']['")
	}
}

func TestConfigResultDirectFirstLevel(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// config('database')['
	source := `<?php
config('database')['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 20})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["default"] {
		t.Error("expected 'default' from config('database')['")
	}
	if !labels["connections"] {
		t.Error("expected 'connections' from config('database')['")
	}
}

func TestConfigResultDirectThreeLevel(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// config('database')['connections']['sqlite']['
	source := `<?php
config('database')['connections']['sqlite']['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 45})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["driver"] {
		t.Error("expected 'driver' from 3-level bracket access")
	}
	if !labels["database"] {
		t.Error("expected 'database' from 3-level bracket access")
	}
}

func TestConfigDotNotationDeepVariableAccess(t *testing.T) {
	p, _ := setupConfigProvider(t)

	// $mysql = config('database.connections.mysql'); $mysql['
	source := `<?php
$mysql = config('database.connections.mysql');
$mysql['
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 8})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' from config('database.connections.mysql') access")
	}
	if !labels["port"] {
		t.Error("expected 'port' from config('database.connections.mysql') access")
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/open-southeners/tusk-php/internal/lsp"
)

var (
	version    = "0.4.0"
	showVer    = flag.Bool("version", false, "Print version and exit")
	logFile    = flag.String("log", "", "Log file path (default: stderr)")
	transport  = flag.String("transport", "stdio", "Transport: stdio")
	stdioMode  = flag.Bool("stdio", false, "Use stdio transport")
	strictMode = flag.Bool("strict", false, "Strict mode: re-panic after recovering (also enabled via TUSK_STRICT env var)")
)

func main() {
	flag.Parse()
	if *stdioMode {
		*transport = "stdio"
	}
	if *showVer {
		fmt.Printf("php-lsp %s\n", version)
		os.Exit(0)
	}
	// Strict mode is enabled via flag OR the TUSK_STRICT environment variable.
	strict := *strictMode || isTruthy(os.Getenv("TUSK_STRICT"))
	var logger *log.Logger
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		logger = log.New(f, "[php-lsp] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "[php-lsp] ", log.LstdFlags|log.Lshortfile)
	}
	logger.Printf("Starting php-lsp %s", version)
	server := lsp.NewServer(os.Stdin, os.Stdout, logger)
	server.SetStrict(strict)
	if err := server.Run(); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// isTruthy returns true when s is one of the common truthy env-var values
// ("1", "true", "yes", "on"), case-insensitively.
func isTruthy(s string) bool {
	switch s {
	case "1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return true
	}
	return false
}

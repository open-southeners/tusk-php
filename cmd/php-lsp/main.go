package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/open-southeners/tusk-php/internal/lsp"
)

var (
	version   = "0.2.0"
	showVer   = flag.Bool("version", false, "Print version and exit")
	logFile   = flag.String("log", "", "Log file path (default: stderr)")
	transport = flag.String("transport", "stdio", "Transport: stdio")
	stdioMode = flag.Bool("stdio", false, "Use stdio transport")
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
	if err := server.Run(); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

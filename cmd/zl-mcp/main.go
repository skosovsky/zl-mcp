package main

import (
	"github.com/skosovsky/zl-mcp/internal/service"
	"os"
)

func main() {
	if err := service.RunCLI(); err != nil {
		service.ReportCLIError(err)
		os.Exit(1)
	}
}

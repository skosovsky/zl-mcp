package main

import (
	"os"

	"github.com/skosovsky/zl-mcp/internal/service"
)

func main() {
	if err := service.RunCLI(); err != nil {
		service.ReportCLIError(err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/KurisuNo1/InterviewAgent/config"
	"github.com/KurisuNo1/InterviewAgent/internal/app"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/cli"
)

func main() {
	configPath := "config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config.LoadEnv(".env")
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Wire all dependencies
	application, err := app.Wire(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize: %v\n", err)
		os.Exit(1)
	}

	// Provide the InterviewService to CLI commands
	cli.InterviewServiceProvider = func() interaction.InterviewService {
		return application.Orchestrator
	}

	cli.Execute()
}

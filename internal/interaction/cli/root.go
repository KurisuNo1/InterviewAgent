package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "interview",
	Short: "InterviewAgent CLI - AI-powered interview system",
	Long: `InterviewAgent CLI provides command-line access to the AI interview system.
It supports creating interview sessions, conducting interviews, and skill practice.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("InterviewAgent CLI - use 'interview --help' for available commands")
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

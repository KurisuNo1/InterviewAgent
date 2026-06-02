package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(skillCmd)
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Practice mode - algorithm, system design, behavioral, tech quiz",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		subIntent := args[0]
		msg := ""
		if len(args) > 1 {
			msg = args[1]
		}

		resp, err := svc.HandleMessage(context.Background(), "", fmt.Sprintf("skill:%s:%s", subIntent, msg))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skill error: %v\n", err)
			return
		}
		fmt.Printf("[%s] %s\n", resp.Intent, resp.Reply)
	},
}

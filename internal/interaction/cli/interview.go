package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/spf13/cobra"
)

// InterviewServiceProvider provides access to the InterviewService.
var InterviewServiceProvider func() interaction.InterviewService

func init() {
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(answerCmd)
	rootCmd.AddCommand(skipCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(reviewPlanCmd)
}

var createCmd = &cobra.Command{
	Use:   "create [jd-text]",
	Short: "Create a new interview session with a job description",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		jdText := ""
		if len(args) > 0 {
			jdText = args[0]
		}

		session, err := svc.CreateSession(context.Background(), interaction.CreateSessionReq{
			JDText: jdText,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
			return
		}
		fmt.Printf("Session created: %s (status: %s)\n", session.ID, session.Status)
	},
}

var startCmd = &cobra.Command{
	Use:   "start [session-id]",
	Short: "Start the interview for a session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		event, err := svc.StartInterview(context.Background(), args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start interview: %v\n", err)
			return
		}
		fmt.Printf("[%s] %v\n", event.Type, event.Data)
	},
}

var answerCmd = &cobra.Command{
	Use:   "answer [session-id] [text]",
	Short: "Submit an answer to the current question",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		event, err := svc.SubmitAnswer(context.Background(), args[0], args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to submit answer: %v\n", err)
			return
		}
		fmt.Printf("[%s] %v\n", event.Type, event.Data)
	},
}

var skipCmd = &cobra.Command{
	Use:   "skip [session-id]",
	Short: "Skip the current question",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		event, err := svc.SkipQuestion(context.Background(), args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to skip question: %v\n", err)
			return
		}
		fmt.Printf("[%s] %v\n", event.Type, event.Data)
	},
}

var reportCmd = &cobra.Command{
	Use:   "report [session-id]",
	Short: "Get the interview evaluation report",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		report, err := svc.GetReport(context.Background(), args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get report: %v\n", err)
			return
		}
		fmt.Printf("Overall Score: %.2f\n", report.OverallScore)
		fmt.Printf("Summary: %s\n", report.Summary)
		if len(report.Highlights) > 0 {
			fmt.Println("\nHighlights:")
			for _, h := range report.Highlights {
				fmt.Printf("  + %s\n", h)
			}
		}
		if len(report.WeakAreas) > 0 {
			fmt.Println("\nWeak Areas:")
			for _, w := range report.WeakAreas {
				fmt.Printf("  - %s\n", w)
			}
		}
	},
}

var reviewPlanCmd = &cobra.Command{
	Use:   "review-plan [session-id]",
	Short: "Get the personalized review plan",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		svc := InterviewServiceProvider()
		if svc == nil {
			fmt.Fprintln(os.Stderr, "InterviewService not available")
			return
		}

		plan, err := svc.GetReviewPlan(context.Background(), args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get review plan: %v\n", err)
			return
		}
		fmt.Println("Review Plan:")
		for _, item := range plan.PlanItems {
			fmt.Printf("  [%s] %s (%.1fh)\n", item.Priority, item.Topic, item.EstimatedHours)
		}
		if len(plan.Resources) > 0 {
			fmt.Println("\nResources:")
			for _, r := range plan.Resources {
				fmt.Printf("  - %s (%s): %s\n", r.Title, r.Type, r.URL)
			}
		}
	},
}

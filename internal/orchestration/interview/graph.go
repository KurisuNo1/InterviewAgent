package interview

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/compose"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/interview/nodes"
)

type Runnable = compose.Runnable[*nodes.InterviewState, *nodes.InterviewState]

type GraphConfig struct{ MaxFollowUps, MaxQuestions int; CheckpointStore compose.CheckPointStore }

type NodeSet struct {
	JDAnalysis       *nodes.JDAnalysisNode
	ResumeMatching   *nodes.ResumeMatchingNode
	QuestionPlanning *nodes.QuestionPlanningNode
	Interviewer      *nodes.InterviewerNode
	Evaluation       *nodes.EvaluationNode
	ReviewPlanning   *nodes.ReviewPlanningNode
}

// CompileSetupGraph builds a simple DAG for the setup phase:
// START → JD Analysis → Resume Matching → Question Planning → END
func CompileSetupGraph(ctx context.Context, ns *NodeSet) (Runnable, error) {
	graph := compose.NewGraph[*nodes.InterviewState, *nodes.InterviewState]()

	if err := graph.AddLambdaNode("jd_analysis",
		compose.InvokableLambda(jdAnalysisLambda(ns.JDAnalysis)), compose.WithNodeName("JD Analysis")); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("resume_matching",
		compose.InvokableLambda(resumeMatchingLambda(ns.ResumeMatching)), compose.WithNodeName("Resume Matching")); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("question_planning",
		compose.InvokableLambda(questionPlanningLambda(ns.QuestionPlanning)), compose.WithNodeName("Question Planning")); err != nil {
		return nil, err
	}

	graph.AddEdge(compose.START, "jd_analysis")
	graph.AddEdge("jd_analysis", "resume_matching")
	graph.AddEdge("resume_matching", "question_planning")
	graph.AddEdge("question_planning", compose.END)

	compiled, err := graph.Compile(ctx, compose.WithGraphName("setup_dag"), compose.WithMaxRunSteps(10))
	if err != nil {
		return nil, fmt.Errorf("setup graph compile failed: %w", err)
	}
	log.Printf("[Graph] Setup DAG compiled successfully")
	return compiled, nil
}

// CompileInterviewGraph builds the full interview DAG with interrupt support:
//
//	START → Interviewer (interrupts for answer) → Evaluation → ReviewPlanning → END
//	         ↑                                        |
//	         └────────── (if more questions) ──────────┘
//
// The Interviewer node uses StatefulInterrupt to pause execution after each question.
// ResumeWithData provides the user's answer, and the graph continues through Evaluation.
// Branching (follow_up/next_question/complete) is handled inside the nodes.
func CompileInterviewGraph(ctx context.Context, ns *NodeSet, checkpointStore compose.CheckPointStore) (Runnable, error) {
	graph := compose.NewGraph[*nodes.InterviewState, *nodes.InterviewState]()

	// Interviewer node with interrupt support
	if err := graph.AddLambdaNode("interviewer",
		compose.InvokableLambda(interviewerInterruptLambda(ns.Interviewer)),
		compose.WithNodeName("Interviewer"),
		compose.WithStatePreHandler(func(ctx context.Context, in *nodes.InterviewState, state string) (*nodes.InterviewState, error) {
			// Transition from question_planning to interviewing if needed
			if in.Phase == "question_planning" {
				in.Phase = "interviewing"
			}
			return in, nil
		}),
	); err != nil {
		return nil, err
	}

	// Evaluation node
	if err := graph.AddLambdaNode("evaluation",
		compose.InvokableLambda(evaluationLambda(ns.Evaluation)),
		compose.WithNodeName("Evaluation"),
	); err != nil {
		return nil, err
	}

	// Review Planning node
	if err := graph.AddLambdaNode("review_planning",
		compose.InvokableLambda(reviewPlanningLambda(ns.ReviewPlanning)),
		compose.WithNodeName("Review Planning"),
	); err != nil {
		return nil, err
	}

	// Edges: START → interviewer → evaluation → review_planning → END
	graph.AddEdge(compose.START, "interviewer")
	graph.AddEdge("interviewer", "evaluation")
	graph.AddEdge("evaluation", "review_planning")
	graph.AddEdge("review_planning", compose.END)

	opts := []compose.GraphCompileOption{
		compose.WithGraphName("interview_dag"),
		compose.WithMaxRunSteps(50),
	}
	if checkpointStore != nil {
		opts = append(opts, compose.WithCheckPointStore(checkpointStore))
	}

	compiled, err := graph.Compile(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("interview graph compile failed: %w", err)
	}
	log.Printf("[Graph] Interview DAG compiled successfully (with interrupt/resume)")
	return compiled, nil
}

// interviewerInterruptLambda creates a lambda that interrupts after asking each question.
func interviewerInterruptLambda(iv *nodes.InterviewerNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}

		// Check if we've exceeded the question queue
		if state.CurrentQIndex >= len(state.QuestionQueue) {
			state.NextAction = "complete"
			return state, nil
		}

		// Ask the current question
		question, err := iv.AskQuestion(ctx, state)
		if err != nil {
			return nil, err
		}

		// Store the question in interrupt data and pause
		state.InterruptData["current_question"] = question
		state.InterruptData["awaiting"] = "answer"

		// Use StatefulInterrupt to pause execution and wait for user input
		return nil, compose.StatefulInterrupt(ctx, "await_answer", state)
	}
}

// evaluationLambda evaluates the answer after resume.
func evaluationLambda(ev *nodes.EvaluationNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}

		// If we just resumed with an answer, the answer is in the state's ChatHistory
		if err := ev.Execute(ctx, state); err != nil {
			// Non-fatal: log and continue
			log.Printf("[Graph] Evaluation warning: %v", err)
		}

		return state, nil
	}
}

// reviewPlanningLambda generates the review plan.
func reviewPlanningLambda(rp *nodes.ReviewPlanningNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}

		if state.Phase == "completed" || state.CurrentQIndex >= len(state.QuestionQueue) {
			if err := rp.Execute(ctx, state); err != nil {
				log.Printf("[Graph] Review planning warning: %v", err)
			}
		}
		return state, nil
	}
}

func jdAnalysisLambda(jda *nodes.JDAnalysisNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}
		if state.JDAnalysis != nil {
			return state, nil
		}
		return state, jda.Execute(ctx, state)
	}
}

func resumeMatchingLambda(rm *nodes.ResumeMatchingNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}
		if state.ResumeMatch != nil {
			return state, nil
		}
		// Skip if resume text hasn't been uploaded yet (e.g. ParseJD called before resume upload)
		resumeText, _ := state.InterruptData["resume_text"].(string)
		if resumeText == "" {
			return state, nil
		}
		return state, rm.Execute(ctx, state)
	}
}

func questionPlanningLambda(qp *nodes.QuestionPlanningNode) func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
	return func(ctx context.Context, state *nodes.InterviewState) (*nodes.InterviewState, error) {
		if state == nil {
			state = &nodes.InterviewState{}
		}
		if state.QuestionPlan != nil && len(state.QuestionQueue) > 0 {
			return state, nil
		}
		return state, qp.Execute(ctx, state)
	}
}

package chat

import (
	"context"
	"sort"

	"slimebot/internal/constants"
	sandboxpolicy "slimebot/internal/sandbox"
	"slimebot/internal/tools"
)

type approvalDecision struct {
	approved         bool
	rejectionMessage string
	answers          string
	status           string
	error            string
	notified         bool
	escalationGrant  bool
}

type parallelToolJob struct {
	index            int
	toolCallID       string
	toolName         string
	command          string
	modelFuncName    string
	requiresApproval bool
	silent           bool
	awaitApproval    func(context.Context) approvalDecision
	execute          func(context.Context) *tools.ExecuteResult
}

type parallelToolOutcome struct {
	index          int
	toolCallID     string
	messageContent string
	status         string
	output         string
	error          string
	metadata       any
	imageURL       string
}

func runParallelToolJobs(
	ctx context.Context,
	jobs []parallelToolJob,
	maxParallel int,
	onResult func(ToolCallResult),
) []parallelToolOutcome {
	if len(jobs) == 0 {
		return nil
	}
	if maxParallel <= 0 {
		maxParallel = constants.MaxParallelToolCalls
	}

	sem := make(chan struct{}, maxParallel)
	outCh := make(chan parallelToolOutcome, len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			if job.requiresApproval && job.awaitApproval != nil {
				decision := job.awaitApproval(ctx)
				if !decision.approved {
					status := decision.status
					if status == "" {
						status = constants.ToolCallStatusRejected
					}
					errText := decision.error
					if errText == "" {
						errText = "Execution was rejected by the user."
					}
					if onResult != nil && !decision.notified && !job.silent {
						onResult(ToolCallResult{
							ToolCallID:       job.toolCallID,
							ToolName:         job.toolName,
							Command:          job.command,
							ModelFuncName:    job.modelFuncName,
							RequiresApproval: job.requiresApproval,
							Status:           status,
							Error:            errText,
						})
					}
					outCh <- parallelToolOutcome{
						index:          job.index,
						toolCallID:     job.toolCallID,
						messageContent: decision.rejectionMessage,
						status:         status,
						error:          errText,
					}
					return
				}
				if decision.escalationGrant {
					ctx = sandboxpolicy.WithEscalationGrant(ctx)
				}
				_ = decision.answers
			}

			select {
			case <-ctx.Done():
				outCh <- parallelToolOutcome{
					index:          job.index,
					toolCallID:     job.toolCallID,
					messageContent: "Approval timed out or failed. The tool call was cancelled.",
					status:         constants.ToolCallStatusError,
					error:          ctx.Err().Error(),
				}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			execResult := job.execute(ctx)
			status := buildToolResultStatus(execResult)
			result := ToolCallResult{
				ToolCallID:       job.toolCallID,
				ToolName:         job.toolName,
				Command:          job.command,
				ModelFuncName:    job.modelFuncName,
				RequiresApproval: job.requiresApproval,
				Status:           status,
			}
			if execResult != nil {
				result.Output = execResult.Output
				result.Error = execResult.Error
				result.Metadata = execResult.Metadata
			}
			if onResult != nil && !job.silent {
				onResult(result)
			}
			imageURL := ""
			if execResult != nil {
				imageURL = execResult.ImageURL
			}
			outCh <- parallelToolOutcome{
				index:          job.index,
				toolCallID:     job.toolCallID,
				messageContent: buildToolResultContent(execResult),
				status:         status,
				output:         result.Output,
				error:          result.Error,
				metadata:       result.Metadata,
				imageURL:       imageURL,
			}
		}()
	}

	outcomes := make([]parallelToolOutcome, 0, len(jobs))
	for range jobs {
		outcomes = append(outcomes, <-outCh)
	}
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].index < outcomes[j].index
	})
	return outcomes
}

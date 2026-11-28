package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"slimebot/internal/constants"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

type specialToolResult struct {
	content string
	image   *llmsvc.ChatMessage
}

type specialToolHandler func(context.Context, toolInvocation) (specialToolResult, error)

func (b *toolCallBatch) runSerial(ctx context.Context, invocation toolInvocation, handler specialToolHandler) error {
	b.flushParallelJobs(ctx)
	if err := b.traceError(); err != nil {
		return err
	}
	return b.handleSpecial(ctx, invocation, handler)
}

func (b *toolCallBatch) handleSpecial(ctx context.Context, invocation toolInvocation, handler specialToolHandler) error {
	result, err := handler(ctx, invocation)
	if err != nil {
		return err
	}
	if result.image != nil {
		b.deferredImages = append(b.deferredImages, *result.image)
	}
	b.appendResult(ctx, invocation.call.ID, result.content)
	return nil
}

func (b *toolCallBatch) handleCooperativeTool(ctx context.Context, invocation toolInvocation) (specialToolResult, error) {
	call := invocation.call
	var params map[string]any
	toolErr := json.Unmarshal([]byte(call.Arguments), &params)
	policy := toolApprovalPolicyNone
	if call.Name == "integrate_artifact" || call.Name == "validate_artifact" {
		policy = determineToolApprovalPolicy(constants.ExecToolName, false, b.opts.ApprovalMode)
	}
	resolved := resolvedToolInvocation{
		toolName: call.Name, command: call.Name, modelFuncName: call.Name,
		requiresApproval: policy != toolApprovalPolicyNone, approvalPolicy: policy,
	}
	if b.callbacks.OnToolCallStart != nil {
		request := (toolInvocation{call: call, resolvedToolInvocation: resolved, params: params}).startRequest()
		request.ModelFuncName = ""
		if err := b.callbacks.OnToolCallStart(request); err != nil {
			b.keepAnswerOnError = true
			return specialToolResult{}, err
		}
	}

	output := ""
	status := constants.ToolCallStatusCompleted
	if toolErr == nil {
		review := func(ctx context.Context, request ApprovalReviewRequest) (*ApprovalReviewResult, error) {
			return b.agent.reviewToolApproval(ctx, b.model, b.messages, request)
		}
		approved, rejection, _ := waitApprovalIfNeeded(ctx, b.callbacks, call, resolved, params, "", review)
		if b.opts.steering.hasPending() {
			approved = false
			rejection = "Skipped after new user guidance."
		}
		if approved {
			output, toolErr = b.opts.Cooperative.execute(ctx, call.Name, call.Arguments)
		} else {
			status = constants.ToolCallStatusRejected
			output = rejection
		}
	}
	if toolErr != nil {
		output = cooperativeError(toolErr)
		status = constants.ToolCallStatusError
	}
	notifyToolResult(b.callbacks, ToolCallResult{
		ToolCallID: call.ID, ToolName: call.Name, Command: call.Name, Status: status, Output: output,
	})
	return specialToolResult{content: output}, nil
}

func (b *toolCallBatch) handleContextRead(_ context.Context, invocation toolInvocation) (specialToolResult, error) {
	content, image, err := executeContextRead(b.contextRun, invocation.call.Arguments)
	if err != nil {
		content = err.Error()
	}
	return specialToolResult{content: content, image: image}, nil
}

func (b *toolCallBatch) handlePlanStart(_ context.Context, _ toolInvocation) (specialToolResult, error) {
	if b.opts.PlanStarted != nil {
		*b.opts.PlanStarted = true
	}
	if b.callbacks.OnPlanStart != nil {
		if err := b.callbacks.OnPlanStart(); err != nil {
			return specialToolResult{}, fmt.Errorf("OnPlanStart callback failed: %w", err)
		}
	}
	return specialToolResult{content: "Plan writing phase started."}, nil
}

func (b *toolCallBatch) handlePlanComplete(_ context.Context, _ toolInvocation) (specialToolResult, error) {
	if b.opts.PlanComplete != nil {
		*b.opts.PlanComplete = true
	}
	return specialToolResult{content: "Plan submitted for review."}, nil
}

func (b *toolCallBatch) handleActivateSkill(ctx context.Context, invocation toolInvocation) (specialToolResult, error) {
	runtime := tools.TurnToolRuntime{SkillRuntime: b.runtime.SkillRuntime, ActivatedSkills: b.runtime.ActivatedSkills}
	result := executeToolCall(tools.WithTurnToolRuntime(ctx, runtime), constants.ActivateSkillTool, "activate", invocation.params)
	if strings.TrimSpace(result.Error) != "" {
		return specialToolResult{content: "failed to activate skill: " + result.Error}, nil
	}
	return specialToolResult{content: result.Output}, nil
}

func (b *toolCallBatch) handleAskQuestions(ctx context.Context, invocation toolInvocation) (specialToolResult, error) {
	if b.callbacks.OnToolCallStart != nil {
		request := invocation.startRequest()
		request.ReviewStatus = ""
		if err := b.callbacks.OnToolCallStart(request); err != nil {
			return specialToolResult{}, fmt.Errorf("failed to push tool approval request: %w", err)
		}
	}
	approved, rejection, answers := waitApprovalIfNeeded(ctx, b.callbacks, invocation.call, invocation.resolvedToolInvocation, invocation.params, "", nil)
	if !approved {
		return specialToolResult{content: rejection}, nil
	}
	formatted := formatAskQuestionsAnswers(fmt.Sprintf("%v", invocation.params["questions"]), answers)
	notifyToolResult(b.callbacks, ToolCallResult{
		ToolCallID: invocation.call.ID, ToolName: invocation.toolName, Command: invocation.command,
		ModelFuncName: invocation.modelFuncName, RequiresApproval: invocation.requiresApproval,
		Status: constants.ToolCallStatusCompleted, Output: formatted,
	})
	return specialToolResult{content: "User answers:\n" + formatted}, nil
}

func formatAskQuestionsAnswers(questionsJSON string, answersJSON string) string {
	type qItem struct {
		ID       string   `json:"id"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	type answer struct {
		QuestionID     string `json:"questionId"`
		SelectedOption int    `json:"selectedOption"`
		CustomAnswer   string `json:"customAnswer"`
	}
	type readableAnswer struct {
		ID       string `json:"id"`
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}

	var questions []qItem
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return answersJSON
	}
	var answers []answer
	if err := json.Unmarshal([]byte(answersJSON), &answers); err != nil {
		return answersJSON
	}

	qMap := make(map[string]qItem, len(questions))
	for _, q := range questions {
		qMap[q.ID] = q
	}

	result := make([]readableAnswer, 0, len(answers))
	for _, a := range answers {
		q, ok := qMap[a.QuestionID]
		if !ok {
			result = append(result, readableAnswer{ID: a.QuestionID, Question: "(unknown)", Answer: a.CustomAnswer})
			continue
		}
		ansText := a.CustomAnswer
		if a.SelectedOption >= 0 && a.SelectedOption < len(q.Options) {
			ansText = q.Options[a.SelectedOption]
		}
		result = append(result, readableAnswer{ID: q.ID, Question: q.Question, Answer: ansText})
	}

	b, err := json.Marshal(result)
	if err != nil {
		return answersJSON
	}
	return string(b)
}

// drainAllProcesses runs at host shutdown without holding the session-state lock.

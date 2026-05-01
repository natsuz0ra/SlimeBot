package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"slimebot/internal/constants"
)

type askQuestionsTool struct{}

type questionItem struct {
	ID                 string   `json:"id"`
	Question           string   `json:"question"`
	Options            []string `json:"options"`
	OptionDescriptions []string `json:"option_descriptions,omitempty"`
}

func init() {
	Register(&askQuestionsTool{})
}

func (a *askQuestionsTool) Name() string { return constants.AskQuestionsTool }

func (a *askQuestionsTool) Description() string {
	return "Ask the user structured clarification questions when the request is ambiguous or has multiple possible interpretations. " +
		"Use this tool when you need to disambiguate user intent before proceeding. " +
		"Each question can have up to 5 preset options; the UI always adds a custom-input option automatically. " +
		"Each option can include an optional description for more context. " +
		"The user's answers will be returned as human-readable text (question and selected answer in plain text)."
}

func (a *askQuestionsTool) Commands() []Command {
	return []Command{
		{
			Name: "ask",
			Description: "Use this tool to ask the user multiple choice questions to clarify ambiguity, gather preferences, confirm decisions, or resolve conflicting requirements. " +
				"Each question can have up to 5 preset options; the UI always provides a custom-input option automatically so the user is never constrained to only presets. " +
				"Use this tool proactively when:\n" +
				"- The user request could be interpreted in multiple ways\n" +
				"- Key details are missing and there are several likely alternatives\n" +
				"- A decision between technical approaches, frameworks, or strategies is needed\n" +
				"- Preferences or configuration choices affect the outcome\n" +
				"Do NOT guess or assume when you could ask instead. Clarifying early prevents rework and leads to better results.",
			Params: []CommandParam{
				{
					Name:        "questions",
					Required:    true,
					Description: "JSON array of questions. Each item: {\"id\":\"unique_id\",\"question\":\"the question text\",\"options\":[\"option1\",\"option2\"],\"option_descriptions\":[\"desc for option1\",\"desc for option2\"]}. Max 5 questions, max 5 options each. option_descriptions is optional; if provided, its length must match options.",
					Example:     `[{"id":"q1","question":"Which framework do you prefer?","options":["React","Vue","Angular"],"option_descriptions":["A declarative component-based UI library","A progressive framework for building UIs","A full-featured enterprise framework"]}]`,
				},
			},
		},
	}
}

func (a *askQuestionsTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "ask":
		return a.ask(params)
	default:
		return nil, fmt.Errorf("ask_questions tool does not support command: %s", command)
	}
}

func (a *askQuestionsTool) ask(params map[string]any) (*ExecuteResult, error) {
	raw := paramStringTrim(params, "questions")
	if raw == "" {
		return nil, fmt.Errorf("questions parameter is required.")
	}

	var questions []questionItem
	if err := json.Unmarshal([]byte(raw), &questions); err != nil {
		return nil, fmt.Errorf("failed to parse questions JSON: %w", err)
	}

	if len(questions) == 0 {
		return nil, fmt.Errorf("at least one question is required.")
	}
	if len(questions) > constants.AskQuestionsMaxQuestions {
		return nil, fmt.Errorf("too many questions: got %d, max %d.", len(questions), constants.AskQuestionsMaxQuestions)
	}

	for i, q := range questions {
		if strings.TrimSpace(q.ID) == "" {
			return nil, fmt.Errorf("question %d: id is required.", i+1)
		}
		if strings.TrimSpace(q.Question) == "" {
			return nil, fmt.Errorf("question %d (%s): question text is required.", i+1, q.ID)
		}
		if len(q.Options) > constants.AskQuestionsMaxOptionsPerQ {
			return nil, fmt.Errorf("question %d (%s): too many options: got %d, max %d.", i+1, q.ID, len(q.Options), constants.AskQuestionsMaxOptionsPerQ)
		}
		if len(q.OptionDescriptions) > 0 && len(q.OptionDescriptions) != len(q.Options) {
			return nil, fmt.Errorf("question %d (%s): option_descriptions length (%d) must match options length (%d) or be omitted.", i+1, q.ID, len(q.OptionDescriptions), len(q.Options))
		}
	}

	return &ExecuteResult{
		Output: "Questions validated successfully. Waiting for user answers.",
	}, nil
}

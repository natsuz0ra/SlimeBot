package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	acpbridge "slimebot/internal/acp"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	sandboxpolicy "slimebot/internal/sandbox"
	chatsvc "slimebot/internal/services/chat"
	sessionsvc "slimebot/internal/services/session"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
)

var _ sdk.Agent = (*bridge)(nil)
var _ sdk.AgentLoader = (*bridge)(nil)

func (b *bridge) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{
		ProtocolVersion:   sdk.ProtocolVersionNumber,
		AgentInfo:         &sdk.Implementation{Name: "slimebot", Title: new("SlimeBot"), Version: b.Version},
		AgentCapabilities: sdk.AgentCapabilities{LoadSession: true, PromptCapabilities: sdk.PromptCapabilities{Image: true, EmbeddedContext: true}},
	}, nil
}

func (b *bridge) initialState(ctx context.Context, cwd string) (sessionState, error) {
	resolved, err := sessionsvc.ValidateWorkingDirectory(cwd)
	if err != nil {
		return sessionState{}, sdk.NewInvalidParams(err.Error())
	}
	models, err := b.Models.List(ctx)
	if err != nil {
		return sessionState{}, err
	}
	preferred := strings.TrimSpace(b.PreferredModel)
	model := chooseModel(models, preferred)
	if preferred != "" && model == "" {
		return sessionState{}, sdk.NewInvalidParams("Unknown or ambiguous SlimeBot model")
	}
	approval := b.ApprovalMode
	if approval == "" {
		approval = constants.ApprovalModeStandard
	}
	return sessionState{cwd: resolved, model: model, mode: "code", thinking: "medium", approval: approval}, nil
}

func chooseModel(models []domain.LLMConfig, preferred string) string {
	if len(models) == 0 {
		return ""
	}
	if preferred == "" {
		return models[0].ID
	}
	for _, model := range models {
		if model.ID == preferred {
			return model.ID
		}
	}
	var id string
	for _, model := range models {
		if strings.EqualFold(model.Name, preferred) {
			if id != "" {
				return ""
			}
			id = model.ID
		}
	}
	return id
}

func (b *bridge) state(id sdk.SessionId) (sessionState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state, ok := b.states[id]
	if !ok {
		return sessionState{}, sdk.NewInvalidParams("Unknown session. Load it before prompting.")
	}
	return state, nil
}

func (b *bridge) modes(state sessionState) *sdk.SessionModeState {
	return &sdk.SessionModeState{CurrentModeId: sdk.SessionModeId(state.mode), AvailableModes: []sdk.SessionMode{
		{Id: "code", Name: "Code"}, {Id: "plan", Name: "Plan"},
	}}
}

func selectOption(id, name, value string, category *sdk.SessionConfigOptionCategory, values []sdk.SessionConfigSelectOption) sdk.SessionConfigOption {
	choices := sdk.SessionConfigSelectOptionsUngrouped(values)
	return sdk.SessionConfigOption{Select: &sdk.SessionConfigOptionSelect{
		Type: "select", Id: sdk.SessionConfigId(id), Name: name, Category: category, CurrentValue: sdk.SessionConfigValueId(value),
		Options: sdk.SessionConfigSelectOptions{Ungrouped: &choices},
	}}
}

func (b *bridge) options(ctx context.Context, state sessionState) ([]sdk.SessionConfigOption, error) {
	models, err := b.Models.List(ctx)
	if err != nil {
		return nil, err
	}
	choices := make([]sdk.SessionConfigSelectOption, 0, len(models))
	for _, model := range models {
		choices = append(choices, sdk.SessionConfigSelectOption{Value: sdk.SessionConfigValueId(model.ID), Name: model.Name})
	}
	mode := sdk.SessionConfigOptionCategoryMode
	thought := sdk.SessionConfigOptionCategoryThoughtLevel
	result := []sdk.SessionConfigOption{
		selectOption("mode", "Mode", state.mode, &mode, []sdk.SessionConfigSelectOption{{Value: "code", Name: "Code"}, {Value: "plan", Name: "Plan"}}),
		selectOption("thinkingLevel", "Thinking level", state.thinking, &thought, []sdk.SessionConfigSelectOption{
			{Value: "off", Name: "Off"}, {Value: "low", Name: "Low"}, {Value: "medium", Name: "Medium"}, {Value: "high", Name: "High"},
		}),
		selectOption("approvalMode", "Approval", state.approval, nil, []sdk.SessionConfigSelectOption{
			{Value: "standard", Name: "Standard"}, {Value: "auto_review", Name: "Auto review"}, {Value: "auto", Name: "Auto"},
		}),
	}
	if len(choices) > 0 {
		category := sdk.SessionConfigOptionCategoryModel
		current := state.model
		if current == "" {
			current = string(choices[0].Value)
		}
		result = append(result, selectOption("model", "Model", current, &category, choices))
	}
	return result, nil
}

func (b *bridge) NewSession(ctx context.Context, req sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	ctx, finish, err := b.request(ctx)
	if err != nil {
		return sdk.NewSessionResponse{}, err
	}
	defer finish()
	state, err := b.initialState(ctx, req.Cwd)
	if err != nil {
		return sdk.NewSessionResponse{}, err
	}
	options, err := b.options(ctx, state)
	if err != nil {
		return sdk.NewSessionResponse{}, err
	}
	session, err := b.Sessions.Create(ctx, "", state.cwd)
	if err != nil {
		return sdk.NewSessionResponse{}, err
	}
	id := sdk.SessionId(session.ID)
	b.mu.Lock()
	b.states[id] = state
	b.mu.Unlock()
	return sdk.NewSessionResponse{SessionId: id, Modes: b.modes(state), ConfigOptions: options}, nil
}

func (b *bridge) LoadSession(ctx context.Context, req sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	ctx, finish, err := b.request(ctx)
	if err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	defer finish()
	state, err := b.initialState(ctx, req.Cwd)
	if err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	history, err := b.Sessions.ReplaySession(ctx, string(req.SessionId))
	if err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	if history.Session.WorkingDirectory != "" && !sameWorkspaceDirectory(history.Session.WorkingDirectory, state.cwd) {
		return sdk.LoadSessionResponse{}, sdk.NewInvalidParams("cwd differs from the saved session workspace")
	}
	options, err := b.options(ctx, state)
	if err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	if err := b.replay(ctx, req.SessionId, history); err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	b.mu.Lock()
	b.states[req.SessionId] = state
	b.mu.Unlock()
	return sdk.LoadSessionResponse{Modes: b.modes(state), ConfigOptions: options}, nil
}

func (b *bridge) SetSessionMode(_ context.Context, req sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	if req.ModeId != "code" && req.ModeId != "plan" {
		return sdk.SetSessionModeResponse{}, sdk.NewInvalidParams("Unknown session mode")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state, ok := b.states[req.SessionId]
	if !ok {
		return sdk.SetSessionModeResponse{}, sdk.NewInvalidParams("Unknown session")
	}
	state.mode = string(req.ModeId)
	b.states[req.SessionId] = state
	return sdk.SetSessionModeResponse{}, nil
}

func (b *bridge) SetSessionConfigOption(ctx context.Context, req sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	ctx, finish, err := b.request(ctx)
	if err != nil {
		return sdk.SetSessionConfigOptionResponse{}, err
	}
	defer finish()
	if req.ValueId == nil {
		return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Expected a string option value")
	}
	input := req.ValueId
	value := string(input.Value)
	switch input.ConfigId {
	case "mode":
		if value != "code" && value != "plan" {
			return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown mode")
		}
	case "model", "modelId":
		models, err := b.Models.List(ctx)
		if err != nil {
			return sdk.SetSessionConfigOptionResponse{}, err
		}
		value = chooseModel(models, value)
		if value == "" {
			return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown model")
		}
	case "thinkingLevel":
		if value != "off" && value != "low" && value != "medium" && value != "high" {
			return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown thinking level")
		}
	case "approvalMode":
		if !ValidApprovalMode(value) {
			return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown approval mode")
		}
	default:
		return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown configuration option")
	}
	b.mu.Lock()
	state, ok := b.states[input.SessionId]
	if !ok {
		b.mu.Unlock()
		return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams("Unknown session")
	}
	switch input.ConfigId {
	case "mode":
		state.mode = value
	case "model", "modelId":
		state.model = value
	case "thinkingLevel":
		state.thinking = value
	case "approvalMode":
		state.approval = value
	}
	b.states[input.SessionId] = state
	b.mu.Unlock()
	options, err := b.options(ctx, state)
	return sdk.SetSessionConfigOptionResponse{ConfigOptions: options}, err
}

func sameWorkspaceDirectory(saved, current string) bool {
	if savedInfo, err := os.Stat(saved); err == nil {
		if currentInfo, err := os.Stat(current); err == nil {
			return os.SameFile(savedInfo, currentInfo)
		}
	}
	relative, err := filepath.Rel(saved, current)
	return err == nil && relative == "."
}

func ValidApprovalMode(value string) bool {
	return value == constants.ApprovalModeStandard || value == constants.ApprovalModeAutoReview || value == constants.ApprovalModeAuto
}

func (b *bridge) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	ctx, finish, err := b.request(ctx)
	if err != nil {
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	}
	defer finish()
	state, err := b.state(req.SessionId)
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	if state.model == "" {
		return sdk.PromptResponse{}, sdk.NewInvalidParams("Configure a SlimeBot model before prompting")
	}
	text, images, err := decodePrompt(req.Prompt)
	if err != nil {
		return sdk.PromptResponse{}, sdk.NewInvalidParams(err.Error())
	}
	id := string(req.SessionId)
	uploads, err := b.Uploads.RegisterLocalFiles(id, images)
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	var attachmentIDs []string
	for _, file := range uploads {
		attachmentIDs = append(attachmentIDs, file.ID)
	}
	defer func() {
		left, _ := b.Uploads.Consume(id, attachmentIDs)
		b.Uploads.Cleanup(left)
	}()
	ctx = constants.WithClientSurface(ctx, constants.ClientSurfaceCLI)
	ctx = sandboxpolicy.WithWorkingDirectory(ctx, state.cwd)
	result, err := b.Chat.HandleChatStream(ctx, id, uuid.NewString(), text, "", state.model, attachmentIDs, state.thinking, state.mode == "plan", "", state.approval, b.callbacks(ctx, req.SessionId))
	if ctx.Err() != nil || (result != nil && result.IsInterrupted) {
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	}
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func decodePrompt(blocks []sdk.ContentBlock) (string, []chatsvc.LocalAttachmentFile, error) {
	const maxImageBytes = 10 << 20
	var text []string
	var images []chatsvc.LocalAttachmentFile
	for _, block := range blocks {
		if image := block.Image; image != nil {
			if len(images) >= 5 || base64.StdEncoding.DecodedLen(len(image.Data)) > maxImageBytes+2 {
				return "", nil, fmt.Errorf("image upload limit exceeded")
			}
			data, err := base64.StdEncoding.DecodeString(image.Data)
			if err != nil || len(data) == 0 || len(data) > maxImageBytes {
				return "", nil, fmt.Errorf("invalid image data")
			}
			mimeType := http.DetectContentType(data)
			ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}[mimeType]
			if ext == "" || !strings.EqualFold(image.MimeType, mimeType) {
				return "", nil, fmt.Errorf("unsupported image type")
			}
			images = append(images, chatsvc.LocalAttachmentFile{
				Name: fmt.Sprintf("image-%d.%s", len(images)+1, ext), MimeType: mimeType, Data: data, InlineImage: true,
			})
			continue
		}
		if value := acpbridge.ContentText(block); strings.TrimSpace(value) != "" {
			text = append(text, value)
		}
	}
	if len(text) == 0 && len(images) == 0 {
		return "", nil, fmt.Errorf("prompt is empty or has no supported content")
	}
	return strings.Join(text, "\n\n"), images, nil
}

// The SDK cancels the registered prompt context before it calls Cancel.
func (*bridge) Cancel(context.Context, sdk.CancelNotification) error { return nil }
func (*bridge) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, sdk.NewMethodNotFound("authenticate")
}
func (*bridge) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, sdk.NewMethodNotFound("logout")
}
func (*bridge) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, sdk.NewMethodNotFound("session/close")
}
func (*bridge) DeleteSession(context.Context, sdk.DeleteSessionRequest) (sdk.DeleteSessionResponse, error) {
	return sdk.DeleteSessionResponse{}, sdk.NewMethodNotFound("session/delete")
}
func (*bridge) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound("session/list")
}
func (*bridge) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound("session/resume")
}

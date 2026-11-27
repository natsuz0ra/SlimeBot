package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmsvc "slimebot/internal/services/llm"
)

type learningDocumentProvider struct {
	name  string
	calls int
}

func (p *learningDocumentProvider) StreamChatWithTools(_ context.Context, _ llmsvc.ModelRuntimeConfig, _ []llmsvc.ChatMessage, _ []llmsvc.ToolDef, callbacks llmsvc.StreamCallbacks) (*llmsvc.StreamResult, error) {
	p.calls++
	document := learnedMarkdown(p.name, "Reusable procedure", "Updated verified steps for "+p.name+".")
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk(document); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText, FinishReason: "stop"}, nil
}

func TestLearningResolvesRequestedTargetInsteadOfFirstActivatedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha")
	writeSkill(t, root, "beta", "Beta")
	alphaBefore, _ := os.ReadFile(filepath.Join(root, "alpha", "SKILL.md"))
	provider := &learningDocumentProvider{name: "beta"}
	runtime := NewSkillRuntimeService(NewFileSystemSkillStore(root), root)
	learning := NewLearningService(llmsvc.NewFactory(provider), runtime)

	skill, err := learning.Learn(context.Background(), LearningInput{
		Model:             llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI, Model: "fake", ContextSize: 32768},
		UserContent:       "更新 beta Skill",
		FinalResponse:     "已补充验证步骤",
		ActivatedSkillIDs: []string{"alpha", "beta"},
	})
	if err != nil || skill == nil || skill.Name != "beta" {
		t.Fatalf("learning selected the wrong target: skill=%#v err=%v", skill, err)
	}
	alphaAfter, _ := os.ReadFile(filepath.Join(root, "alpha", "SKILL.md"))
	if string(alphaAfter) != string(alphaBefore) {
		t.Fatal("unrelated activated skill was overwritten")
	}
	betaAfter, _ := os.ReadFile(filepath.Join(root, "beta", "SKILL.md"))
	if !strings.Contains(string(betaAfter), "Updated verified steps for beta") {
		t.Fatalf("requested skill was not updated: %s", betaAfter)
	}
	if provider.calls != 2 {
		t.Fatalf("existing target should be regenerated against current content, calls=%d", provider.calls)
	}
}

func TestLearningCreateDoesNotOverwriteActivatedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha")
	before, _ := os.ReadFile(filepath.Join(root, "alpha", "SKILL.md"))
	provider := &learningDocumentProvider{name: "gamma"}
	runtime := NewSkillRuntimeService(NewFileSystemSkillStore(root), root)
	learning := NewLearningService(llmsvc.NewFactory(provider), runtime)

	skill, err := learning.Learn(context.Background(), LearningInput{
		Model:             llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI, Model: "fake", ContextSize: 32768},
		UserContent:       "创建一个新的 gamma Skill",
		ActivatedSkillIDs: []string{"alpha"},
	})
	if err != nil || skill == nil || skill.Name != "gamma" {
		t.Fatalf("new skill was not created: skill=%#v err=%v", skill, err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "alpha", "SKILL.md"))
	if string(after) != string(before) {
		t.Fatal("creating a skill overwrote the activated skill")
	}
}

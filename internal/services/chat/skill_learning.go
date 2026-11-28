package chat

import (
	"context"
	"regexp"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	llmsvc "slimebot/internal/services/llm"
	skillsvc "slimebot/internal/services/skill"
)

type SkillLearningNotice struct {
	SkillID string `json:"skillId,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

var explicitSkillAction = regexp.MustCompile(`(?is)(保存|存储|沉淀|生成|创建|更新|修正|修改).{0,16}(skill|技能)|(skill|技能).{0,12}(保存|存储|生成|创建|更新|修正)|((save|update|create|write|improve).{0,32}skill)|(skill.{0,24}(save|update|create|write|improve))`)
var explicitSkillHabit = regexp.MustCompile(`(?is)以后.{0,16}(同样|一样|这样).{0,8}(处理|执行|做)`)
var negatedSkillAction = regexp.MustCompile(`(?is)(不要|不用|别|禁止|无需|不需要).{0,12}(保存|存储|生成|创建|更新|修正|修改)?.{0,12}(skill|技能)|(do not|don't|avoid).{0,24}(save|update|create|write).{0,16}skill`)

func explicitSkillLearningRequest(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || negatedSkillAction.MatchString(content) {
		return false
	}
	return explicitSkillAction.MatchString(content) || explicitSkillHabit.MatchString(content)
}

func (s *ChatService) maybeLearnSkill(ctx context.Context, sessionID, requestID, assistantMessageID string, state *chatTurnState, result *chatTurnResult, planMode bool) *SkillLearningNotice {
	if planMode || result.executionErr != nil || result.interrupted {
		return nil
	}
	if state.session == nil || (state.session.Kind != "" && state.session.Kind != domain.SessionKindChat) || constants.IsMessagePlatformSessionID(sessionID) {
		return nil
	}
	if !explicitSkillLearningRequest(state.userContent) || !s.beginSkillLearningTurn(sessionID, state.userMessageID) {
		return nil
	}
	notice := &SkillLearningNotice{}
	learnCtx, cancel := context.WithTimeout(llmsvc.WithPrivacyScope(ctx, sessionID, requestID, "skill_learning"), 2*time.Minute)
	defer cancel()
	records, err := s.store.ListSessionToolCallRecordsByAssistantMessageIDs(learnCtx, sessionID, []string{assistantMessageID})
	if err == nil {
		var skill *domain.Skill
		skill, err = s.skillLearning.Learn(learnCtx, skillsvc.LearningInput{
			SessionID: sessionID, RequestID: requestID, Model: state.modelConfig,
			UserContent: state.userContent, FinalResponse: StripContentMarkers(result.answer),
			ToolCalls: records, ActivatedSkillIDs: state.activatedSkillIDs,
		})
		if err == nil && skill != nil {
			s.invalidateSkillActivation(skill.ID)
			notice.SkillID = skill.ID
			notice.Status = "completed"
			notice.Message = "Skill 已写入本地目录，下一轮可重新激活使用。"
			logging.Info("skill_learning_completed", "session", sessionID, "skill", skill.ID)
			return notice
		}
	}
	notice.Status = "failed"
	notice.Message = "Skill 学习写入未完成；主会话回答不受影响。"
	logging.Warn("skill_learning_failed", "session", sessionID, "err", err)
	return notice
}

func (s *ChatService) beginSkillLearningTurn(sessionID, userMessageID string) bool {
	key := sessionID + "/" + userMessageID
	s.skillLearningMu.Lock()
	defer s.skillLearningMu.Unlock()
	if s.skillLearningTurns == nil {
		s.skillLearningTurns = map[string]time.Time{}
	}
	if _, exists := s.skillLearningTurns[key]; exists {
		return false
	}
	s.skillLearningTurns[key] = time.Now()
	if len(s.skillLearningTurns) > 1024 {
		removed := 0
		for candidate := range s.skillLearningTurns {
			if candidate == key {
				continue
			}
			delete(s.skillLearningTurns, candidate)
			removed++
			if removed >= 256 {
				break
			}
		}
	}
	return true
}

func (s *ChatService) invalidateSkillActivation(skillID string) {
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	for _, activated := range s.skillsBySess {
		delete(activated, skillID)
	}
}

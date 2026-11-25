package chat

import "testing"

func TestExplicitSkillLearningRequest(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"把这次排查过程保存为 Skill", true},
		{"更新现有 skill，加入回归验证步骤", true},
		{"Create a reusable skill from this fix", true},
		{"以后按同样方式处理这类问题", true},
		{"不要保存 Skill", false},
		{"不用生成技能，只回答即可", false},
		{"Don't create a skill from this", false},
		{"请解释一下这个 Skill 的用法", false},
		{"以后我们再讨论", false},
		{"", false},
	}
	for _, test := range tests {
		if got := explicitSkillLearningRequest(test.content); got != test.want {
			t.Errorf("explicitSkillLearningRequest(%q)=%v, want %v", test.content, got, test.want)
		}
	}
}

func TestSkillLearningTurnDeduplicatesUserMessage(t *testing.T) {
	service := &ChatService{}
	if !service.beginSkillLearningTurn("session", "user-message") {
		t.Fatal("first explicit turn was not accepted")
	}
	if service.beginSkillLearningTurn("session", "user-message") {
		t.Fatal("same user message triggered learning twice")
	}
	if !service.beginSkillLearningTurn("session", "next-user-message") {
		t.Fatal("new user message was incorrectly deduplicated")
	}
}

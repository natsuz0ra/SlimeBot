package domain

// SkillStore 技能存储接口。
type SkillStore interface {
	ListSkills() ([]Skill, error)
	GetSkillByName(name string) (*Skill, error)
	GetSkillByID(id string) (*Skill, error)
	CreateSkill(item Skill) (*Skill, error)
	DeleteSkill(id string) error
}

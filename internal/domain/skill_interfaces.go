package domain

// SkillStore 负责从 skills 配置目录读取和删除技能目录元数据。
type SkillStore interface {
	ListSkills() ([]Skill, error)
	GetSkillByName(name string) (*Skill, error)
	GetSkillByID(id string) (*Skill, error)
	CreateSkill(item Skill) (*Skill, error)
	DeleteSkill(id string) error
}

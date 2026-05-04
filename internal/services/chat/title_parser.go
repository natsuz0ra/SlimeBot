package chat

type titleStreamParser struct{}

func newTitleStreamParser() *titleStreamParser {
	return &titleStreamParser{}
}

func (p *titleStreamParser) Feed(chunk string) string {
	return chunk
}

func (p *titleStreamParser) Flush() string {
	return ""
}

func (p *titleStreamParser) BeginAssistantTurn() string {
	return ""
}

package agent

import "strings"

type PromptBuilder struct {
	System string
}

func (b PromptBuilder) Build(history []Message) []Message {
	messages := make([]Message, 0, len(history)+1)
	if strings.TrimSpace(b.System) != "" {
		messages = append(messages, Message{
			Role:    RoleSystem,
			Content: strings.TrimSpace(b.System),
		})
	}
	messages = append(messages, history...)
	return messages
}

func CompactSystemPrompt(agentName string, instructions string) string {
	var builder strings.Builder
	if agentName != "" {
		builder.WriteString("You are ")
		builder.WriteString(agentName)
		builder.WriteString(".\n")
	}
	builder.WriteString("Work quickly and call tools only when they are useful. ")
	builder.WriteString("If a tool fails, use the tool error to recover or answer with the limitation.\n")
	builder.WriteString(strings.TrimSpace(instructions))
	return strings.TrimSpace(builder.String())
}

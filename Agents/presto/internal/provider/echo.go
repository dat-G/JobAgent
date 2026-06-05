package provider

import (
	"context"

	"presto/internal/agent"
)

type Echo struct{}

func (Echo) Chat(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		switch msg.Role {
		case agent.RoleTool:
			return agent.ChatResponse{
				Message: agent.Message{Role: agent.RoleAssistant, Content: "tool result: " + msg.Content},
			}, nil
		case agent.RoleUser:
			return agent.ChatResponse{
				Message: agent.Message{Role: agent.RoleAssistant, Content: "presto mock: " + msg.Content},
			}, nil
		}
	}
	return agent.ChatResponse{
		Message: agent.Message{Role: agent.RoleAssistant, Content: "presto mock ready"},
	}, nil
}

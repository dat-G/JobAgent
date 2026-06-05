package workflow

import (
	"fmt"

	"presto/internal/agent"
)

type StepSpec struct {
	Name   string
	Agents []AgentSpec
}

type AgentSpec struct {
	Name     string
	Runner   *agent.Runner
	Prompt   PromptFunc
	Output   OutputContract
	Attempts int
	Session  string
}

func Step(name string, agents ...AgentSpec) StepSpec {
	return StepSpec{Name: name, Agents: agents}
}

func Fixed(steps ...StepSpec) (*Workflow, error) {
	root, err := FixedNode(steps...)
	if err != nil {
		return nil, err
	}
	return New(root)
}

func FixedNode(steps ...StepSpec) (Node, error) {
	nodes := make([]Node, 0, len(steps))
	for stepIndex, step := range steps {
		if len(step.Agents) == 0 {
			return nil, fmt.Errorf("step %d %q must include at least one agent", stepIndex, step.Name)
		}
		stepNodes := make([]Node, 0, len(step.Agents))
		for agentIndex, spec := range step.Agents {
			if spec.Runner == nil {
				return nil, fmt.Errorf("step %d agent %d %q runner is required", stepIndex, agentIndex, spec.Name)
			}
			options := make([]AgentOption, 0, 4)
			if spec.Prompt != nil {
				options = append(options, WithPrompt(spec.Prompt))
			}
			if spec.Output.Enabled() {
				options = append(options, WithOutputContract(spec.Output))
			}
			if spec.Attempts > 0 {
				options = append(options, WithAttempts(spec.Attempts))
			}
			if spec.Session != "" {
				options = append(options, WithSession(spec.Session))
			}
			stepNodes = append(stepNodes, Agent(spec.Name, spec.Runner, options...))
		}
		if len(stepNodes) == 1 {
			nodes = append(nodes, stepNodes[0])
			continue
		}
		nodes = append(nodes, Parallel(stepNodes...))
	}
	return Sequence(nodes...), nil
}

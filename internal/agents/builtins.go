package agents

import copilot "github.com/github/copilot-sdk/go"

const (
	ModeBuild = "build"
	ModePlan  = "plan"
)

func Builtins() []copilot.CustomAgentConfig {
	return []copilot.CustomAgentConfig{
		{
			Name:        ModePlan,
			DisplayName: "Plan",
			Description: "Researches the codebase and builds implementation plans using read-only tools.",
			Tools:       []string{"view", "grep", "glob", "read_file", "list_dir"},
			Prompt:      "You are a planning agent. Explore carefully, summarize findings, use read-only tools, and avoid mutating the workspace.",
		},
		{
			Name:        ModeBuild,
			DisplayName: "Build",
			Description: "Implements changes, mutates the workspace when needed, runs tools, and executes development tasks.",
			Prompt:      "You are a build agent. Make minimal correct changes, use the available file editing and patch tools whenever you need to modify files, run development tools to verify work, mutate the workspace when needed to complete the task, and report concrete outcomes.",
		},
	}
}

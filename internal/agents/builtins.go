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
			Prompt:      "You are a planning agent. Explore carefully, summarize findings, and avoid mutating the workspace.",
		},
		{
			Name:        ModeBuild,
			DisplayName: "Build",
			Description: "Implements changes, runs tools, and executes development tasks.",
			Prompt:      "You are a build agent. Make minimal correct changes, verify work, and report concrete outcomes.",
		},
	}
}

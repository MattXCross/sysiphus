package app

import "strings"

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func helpText() string {
	return "Keys\n" +
		"Enter send prompt or run a slash command\n" +
		"/model opens a model picker and switches model in the current session\n" +
		"Tab switch between build and plan and start a fresh session\n" +
		"Up and Down scroll the transcript pane by line\n" +
		"PgUp and PgDn scroll the transcript pane\n" +
		"Ctrl+Up and Ctrl+Down scroll the transcript one line\n" +
		"Alt+Up and Alt+Down scroll the info pane one line\n" +
		"Alt+PgUp and Alt+PgDn scroll the info pane\n" +
		"Shift+Up and Shift+Down scroll the activity pane one line\n" +
		"Shift+PgUp and Shift+PgDn scroll the activity pane\n" +
		"Mouse wheel scrolls the pane under the cursor\n" +
		"Ctrl+N start a new session\n" +
		"Ctrl+R resume the latest session\n" +
		"Ctrl+P toggle plan/build mode for new sessions\n" +
		"Ctrl+A toggle approval policy between conservative and allow-all\n" +
		"Conservative allows normal edits and dev tools but still blocks high-risk system commands\n" +
		"Ctrl+L clear local transcript and activity panes\n" +
		"\nCommands\n" +
		"/new start a fresh session\n" +
		"/resume resume the latest saved session\n" +
		"/sessions open the current directory session picker\n" +
		"/plan switch to the plan agent and start a fresh session\n" +
		"/build switch to the build agent and start a fresh session\n" +
		"/mode build|plan switch mode and start a fresh session\n" +
		"/model choose a model for the current session\n" +
		"/approvals conservative|allow-all set approval behavior\n" +
		"/clear clear local panes\n" +
		"/quit exit sysiphus\n" +
		"/help show this help\n" +
		"Ctrl+C quit immediately"
}

func joinNonEmpty(items []string, sep string) string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	return strings.Join(filtered, sep)
}

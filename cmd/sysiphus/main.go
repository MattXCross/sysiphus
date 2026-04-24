package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matt/sysiphus/internal/app"
)

func main() {
	p := tea.NewProgram(app.New(), tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sysiphus: %v\n", err)
		os.Exit(1)
	}
}

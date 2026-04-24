package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	wd, err := os.Getwd()
	if err != nil {
		fail("get working directory", err)
	}

	client := copilot.NewClient(&copilot.ClientOptions{Cwd: wd, LogLevel: "error"})
	if err := client.Start(ctx); err != nil {
		fail("start client", err)
	}
	defer client.Stop()

	models, err := client.ListModels(ctx)
	if err != nil {
		fail("list models", err)
	}
	if len(models) < 2 {
		fail("model switch smoke", fmt.Errorf("need at least two available models"))
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:          "sysiphus-model-smoke",
		Model:               models[0].ID,
		WorkingDirectory:    wd,
		Streaming:           true,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	})
	if err != nil {
		fail("create session", err)
	}
	defer session.Disconnect()

	if err := session.SetModel(ctx, models[1].ID, nil); err != nil {
		fail("set model", err)
	}

	fmt.Printf("session: %s\n", session.SessionID)
	fmt.Printf("from: %s\n", models[0].ID)
	fmt.Printf("to: %s\n", models[1].ID)
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}

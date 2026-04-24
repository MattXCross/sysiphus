package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := copilot.NewClient(&copilot.ClientOptions{
		Cwd:      mustGetwd(),
		LogLevel: "error",
	})
	if err := client.Start(ctx); err != nil {
		fail("start client", err)
	}
	defer client.Stop()

	model, err := resolveModel(ctx, client)
	if err != nil {
		fail("resolve model", err)
	}

	var message strings.Builder
	idle := make(chan struct{}, 1)

	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:          "sysiphus-smoke",
		Model:               model,
		WorkingDirectory:    mustGetwd(),
		Streaming:           true,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
		OnEvent: func(event copilot.SessionEvent) {
			switch data := event.Data.(type) {
			case *copilot.AssistantMessageDeltaData:
				message.WriteString(data.DeltaContent)
			case *copilot.AssistantMessageData:
				if message.Len() == 0 {
					message.WriteString(data.Content)
				}
			case *copilot.SessionErrorData:
				fail("session error", fmt.Errorf("%s", data.Message))
			case *copilot.SessionIdleData:
				select {
				case idle <- struct{}{}:
				default:
				}
			}
		},
	})
	if err != nil {
		fail("create session", err)
	}
	defer session.Disconnect()

	_, err = session.Send(ctx, copilot.MessageOptions{
		Prompt: "Reply with exactly: SYSIPHUS_OK",
	})
	if err != nil {
		fail("send prompt", err)
	}

	select {
	case <-idle:
	case <-ctx.Done():
		fail("wait for response", ctx.Err())
	}

	response := strings.TrimSpace(message.String())
	if response == "" {
		fail("response validation", fmt.Errorf("empty response"))
	}

	fmt.Printf("session: %s\n", session.SessionID)
	fmt.Printf("model: %s\n", model)
	fmt.Printf("response: %s\n", response)
	if response != "SYSIPHUS_OK" {
		os.Exit(2)
	}
}

func resolveModel(ctx context.Context, client *copilot.Client) (string, error) {
	models, err := client.ListModels(ctx)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no models available")
	}
	preferred := []string{"gpt-5", "gpt-4.1", "claude-sonnet-4.5", "claude-3.7-sonnet", "gemini-2.5-pro"}
	available := make(map[string]struct{}, len(models))
	for _, model := range models {
		available[model.ID] = struct{}{}
	}
	for _, candidate := range preferred {
		if _, ok := available[candidate]; ok {
			return candidate, nil
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models[0].ID, nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		fail("get working directory", err)
	}
	return wd
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
	os.Exit(1)
}

package agent

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

const minCodexDebugModelsVersion = "0.122.0"

var codexDebugModelsArgs = []string{"debug", "models", "--bundled"}

type codexDebugModelsResponse struct {
	Models []codexDebugModel `json:"models"`
}

type codexDebugModel struct {
	Slug                    string                     `json:"slug"`
	DisplayName             string                     `json:"display_name"`
	Visibility              string                     `json:"visibility"`
	DefaultReasoningLevel   string                     `json:"default_reasoning_level"`
	SupportedReasoningLevel []codexDebugReasoningLevel `json:"supported_reasoning_levels"`
}

type codexDebugReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func discoverCodexModels(ctx context.Context, executablePath string) []Model {
	if executablePath == "" {
		executablePath = "codex"
	}
	version, err := DetectVersion(ctx, executablePath)
	if err != nil || !codexSupportsDebugModels(version) {
		return codexStaticModels()
	}
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executablePath, codexDebugModelsArgs...)
	hideAgentWindow(cmd)
	raw, err := cmd.Output()
	if err != nil {
		return codexStaticModels()
	}
	models, err := parseCodexModelCatalog(raw)
	if err != nil || len(models) == 0 {
		return codexStaticModels()
	}
	return models
}

func codexSupportsDebugModels(version string) bool {
	got, err := parseSemver(version)
	if err != nil {
		return false
	}
	minimum, err := parseSemver(minCodexDebugModelsVersion)
	if err != nil {
		return false
	}
	return !got.lessThan(minimum)
}

func parseCodexModelCatalog(raw []byte) ([]Model, error) {
	var response codexDebugModelsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(response.Models))
	for _, item := range response.Models {
		slug := strings.TrimSpace(item.Slug)
		if slug == "" || item.Visibility == "hide" {
			continue
		}
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = slug
		}
		models = append(models, Model{
			ID:       slug,
			Label:    label,
			Provider: "openai",
			Thinking: codexThinkingFromDebugModel(item),
		})
	}
	if len(models) > 0 {
		models[0].Default = true
	}
	return models, nil
}

func codexThinkingFromDebugModel(model codexDebugModel) *ModelThinking {
	labels := map[string]string{
		"none": "None", "minimal": "Minimal", "low": "Low",
		"medium": "Medium", "high": "High", "xhigh": "Extra high",
		"max": "Max", "ultra": "Ultra",
	}
	levels := make([]ThinkingLevel, 0, len(model.SupportedReasoningLevel))
	for _, item := range model.SupportedReasoningLevel {
		value := strings.TrimSpace(item.Effort)
		if value == "" {
			continue
		}
		label := labels[value]
		if label == "" {
			label = value
		}
		levels = append(levels, ThinkingLevel{Value: value, Label: label, Description: item.Description})
	}
	if len(levels) == 0 {
		return nil
	}
	return &ModelThinking{SupportedLevels: levels, DefaultLevel: model.DefaultReasoningLevel}
}

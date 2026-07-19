package agent

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	runtimeModelsConfigEnv     = "MULTICA_RUNTIME_MODELS_CONFIG"
	runtimeModelsProfileEnv    = "MULTICA_PROFILE"
	runtimeModelsConfigVersion = 1
	runtimeModelsConfigName    = "runtime-models.json"
	runtimeModelsConfigMaxSize = 4 << 20
)

//go:embed runtime_models.json
var defaultRuntimeModelsConfig []byte

type runtimeModelsConfig struct {
	Version   int                                  `json:"version"`
	Providers map[string]runtimeModelsProviderSpec `json:"providers"`
}

type runtimeModelsProviderSpec struct {
	Models []Model `json:"models"`
}

// configuredRuntimeModels reads and validates the external catalog on every
// call. This is intentionally uncached: editing the text file must affect the
// next runtime model-list request without rebuilding or restarting the daemon.
func configuredRuntimeModels(providerType string) ([]Model, error) {
	path, err := runtimeModelsConfigPath()
	if err != nil {
		return nil, err
	}
	raw, err := readRuntimeModelsConfig(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime models config %s: %w", path, err)
	}
	cfg, err := parseRuntimeModelsConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("parse runtime models config %s: %w", path, err)
	}
	spec, ok := cfg.Providers[providerType]
	if !ok {
		return nil, fmt.Errorf("runtime models config %s has no provider %q", path, providerType)
	}
	return cloneModels(spec.Models), nil
}

func readRuntimeModelsConfig(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, runtimeModelsConfigMaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > runtimeModelsConfigMaxSize {
		return nil, fmt.Errorf("file exceeds %d bytes", runtimeModelsConfigMaxSize)
	}
	return raw, nil
}

// runtimeModelsConfigPath resolves an explicit override or initializes the
// profile-local user-editable default. The initial content is written to a
// temporary file and renamed so concurrent readers never observe partial JSON.
func runtimeModelsConfigPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv(runtimeModelsConfigEnv)); explicit != "" {
		path, err := expandHomePath(explicit)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("%s must be an absolute path", runtimeModelsConfigEnv)
		}
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for runtime models config: %w", err)
	}
	dir := filepath.Join(home, ".multica")
	if profile := strings.TrimSpace(os.Getenv(runtimeModelsProfileEnv)); profile != "" {
		if profile != filepath.Base(profile) || profile == "." || profile == ".." {
			return "", fmt.Errorf("invalid %s value %q", runtimeModelsProfileEnv, profile)
		}
		dir = filepath.Join(dir, "profiles", profile)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create runtime models config directory: %w", err)
	}
	path := filepath.Join(dir, runtimeModelsConfigName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat runtime models config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".runtime-models-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary runtime models config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("set runtime models config permissions: %w", err)
	}
	if _, err := tmp.Write(defaultRuntimeModelsConfig); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("initialize runtime models config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close runtime models config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// On platforms where rename cannot replace an existing file, another
		// concurrent initializer may have won. Its atomically published file is
		// equivalent, so treat that case as success.
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
		return "", fmt.Errorf("publish runtime models config: %w", err)
	}
	return path, nil
}

func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand runtime models config path: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func parseRuntimeModelsConfig(raw []byte) (*runtimeModelsConfig, error) {
	var cfg runtimeModelsConfig
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values are not allowed")
		}
		return nil, err
	}
	if cfg.Version != runtimeModelsConfigVersion {
		return nil, fmt.Errorf("unsupported version %d (want %d)", cfg.Version, runtimeModelsConfigVersion)
	}
	if len(cfg.Providers) == 0 {
		return nil, errors.New("providers must not be empty")
	}
	for provider, spec := range cfg.Providers {
		if strings.TrimSpace(provider) == "" {
			return nil, errors.New("provider name must not be empty")
		}
		if provider != strings.TrimSpace(provider) {
			return nil, fmt.Errorf("provider name %q has surrounding whitespace", provider)
		}
		if err := validateConfiguredModels(provider, spec.Models); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func validateConfiguredModels(provider string, models []Model) error {
	seenModels := make(map[string]struct{}, len(models))
	defaultCount := 0
	for i, model := range models {
		trimmedID := strings.TrimSpace(model.ID)
		trimmedLabel := strings.TrimSpace(model.Label)
		if trimmedID == "" || trimmedLabel == "" {
			return fmt.Errorf("provider %q model %d requires non-empty id and label", provider, i)
		}
		if model.ID != trimmedID || model.Label != trimmedLabel {
			return fmt.Errorf("provider %q model %d id and label must not have surrounding whitespace", provider, i)
		}
		if model.Provider != strings.TrimSpace(model.Provider) {
			return fmt.Errorf("provider %q model %q provider field has surrounding whitespace", provider, model.ID)
		}
		if _, exists := seenModels[model.ID]; exists {
			return fmt.Errorf("provider %q has duplicate model id %q", provider, model.ID)
		}
		seenModels[model.ID] = struct{}{}
		if model.Default {
			defaultCount++
		}
		if model.Thinking == nil {
			continue
		}
		seenLevels := make(map[string]struct{}, len(model.Thinking.SupportedLevels))
		for _, level := range model.Thinking.SupportedLevels {
			trimmedValue := strings.TrimSpace(level.Value)
			trimmedLabel := strings.TrimSpace(level.Label)
			if trimmedValue == "" || trimmedLabel == "" {
				return fmt.Errorf("provider %q model %q has thinking level with empty value or label", provider, model.ID)
			}
			if level.Value != trimmedValue || level.Label != trimmedLabel {
				return fmt.Errorf("provider %q model %q thinking level value and label must not have surrounding whitespace", provider, model.ID)
			}
			if _, exists := seenLevels[level.Value]; exists {
				return fmt.Errorf("provider %q model %q has duplicate thinking level %q", provider, model.ID, level.Value)
			}
			seenLevels[level.Value] = struct{}{}
		}
		if defaultLevel := strings.TrimSpace(model.Thinking.DefaultLevel); defaultLevel != "" {
			if _, exists := seenLevels[defaultLevel]; !exists {
				return fmt.Errorf("provider %q model %q default thinking level %q is unsupported", provider, model.ID, defaultLevel)
			}
		}
	}
	if defaultCount > 1 {
		return fmt.Errorf("provider %q has %d default models; at most one is allowed", provider, defaultCount)
	}
	return nil
}

func defaultConfiguredModels(providerType string) []Model {
	cfg, err := parseRuntimeModelsConfig(defaultRuntimeModelsConfig)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded runtime models config: %v", err))
	}
	return cloneModels(cfg.Providers[providerType].Models)
}

func cloneModels(models []Model) []Model {
	if models == nil {
		return []Model{}
	}
	out := make([]Model, len(models))
	for i, model := range models {
		out[i] = model
		if model.Thinking != nil {
			thinking := *model.Thinking
			thinking.SupportedLevels = append([]ThinkingLevel(nil), model.Thinking.SupportedLevels...)
			out[i].Thinking = &thinking
		}
	}
	return out
}

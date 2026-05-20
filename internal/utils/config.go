package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sokinpui/chat2response/internal/types"
)

func LoadConfigFile(searchFrom string) (*types.ConfigFile, error) {
	configPath := findConfigPath(searchFrom)
	if configPath == "" {
		return nil, nil
	}

	if filepath.Ext(configPath) != ".json" {
		return nil, fmt.Errorf("only .json config files are supported in the Go version")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config types.ConfigFile
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func findConfigPath(searchFrom string) string {
	currentDir, err := filepath.Abs(searchFrom)
	if err != nil {
		return ""
	}

	for {
		for _, name := range types.ConfigFileNames {
			candidate := filepath.Join(currentDir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return ""
}

func ValidateConfig(config *types.ConfigFile) error {
	if config == nil {
		return errors.New("config is nil")
	}

	if config.Version == "" {
		return errors.New("config must have a version string")
	}

	if config.CurrentUpstream == "" {
		return errors.New("config must have a currentUpstream string")
	}

	upstream, ok := config.Upstreams[config.CurrentUpstream]
	if !ok {
		return fmt.Errorf("currentUpstream %q not found in upstreams", config.CurrentUpstream)
	}

	for name, cfg := range config.Upstreams {
		if err := ValidateUpstreamConfig(&cfg); err != nil {
			return fmt.Errorf("upstream %q is invalid: %w", name, err)
		}
	}

	return nil
}

func ValidateUpstreamConfig(upstream *types.UpstreamConfig) error {
	if upstream.Format != "" {
		valid := false
		formats := []types.UpstreamFormat{types.UpstreamFormatAnthropic, types.UpstreamFormatOpenAIChat}
		for _, f := range formats {
			if upstream.Format == f {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid format: %s", upstream.Format)
		}
	}

	if upstream.BaseURL == "" {
		return errors.New("baseUrl is required")
	}

	return nil
}

func GetCurrentUpstreamConfig(config *types.ConfigFile) *types.UpstreamConfig {
	if config == nil {
		return nil
	}
	upstream, ok := config.Upstreams[config.CurrentUpstream]
	if !ok {
		return nil
	}
	return &upstream
}

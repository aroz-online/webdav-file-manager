package main

import (
	"encoding/json"
	"os"
)

const configFilePath = "./config.json"

// DefaultConfig returns the default configuration
func DefaultConfig() WebDAVClientConfig {
	return WebDAVClientConfig{
		Enabled:       true,
		Port:          5488,
		Username:      "",
		Password:      "",
		MaxUploadSize: 25 * 1024 * 1024, // 25MB
	}
}

// LoadConfig loads the configuration from the config file
func LoadConfig() (WebDAVClientConfig, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config on first run
			_ = SaveConfig(cfg)
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	return cfg, nil
}

// SaveConfig saves the configuration to the config file with restricted permissions
func SaveConfig(cfg WebDAVClientConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Write with 0600 permissions (owner read/write only)
	return os.WriteFile(configFilePath, data, 0600)
}

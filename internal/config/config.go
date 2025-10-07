package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DataDir string       `toml:"data_dir"`
	Repos   []RepoConfig `toml:"repos"`
}

type RepoConfig struct {
	Owner        string   `toml:"owner"`
	Name         string   `toml:"name"`
	BuildCommand []string `toml:"build_command"`
	// Name mapped to "encrypted_build_secrets" - we decrypt it as part of loading the config
	BuildSecrets map[string]string `toml:"encrypted_build_secrets"`
	// Name mapped to "encrypted_webhook_secret" - we decrypt it as part of loading the config
	WebhookSecret *string `toml:"encrypted_webhook_secret,omitempty"`
}

func Load(secretKey, configFile string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(configFile, &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Decrypt secrets
	for i := range cfg.Repos {
		for secretName := range cfg.Repos[i].BuildSecrets {
			plaintext, err := decryptSecret(secretKey, cfg.Repos[i].BuildSecrets[secretName])
			if err != nil {
				return nil, fmt.Errorf(
					"failed to decrypt build secret of %s/%s: %w",
					cfg.Repos[i].Owner, cfg.Repos[i].Name, err,
				)
			}

			cfg.Repos[i].BuildSecrets[secretName] = plaintext
		}

		if cfg.Repos[i].WebhookSecret != nil {
			plaintext, err := decryptSecret(secretKey, *cfg.Repos[i].WebhookSecret)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to decrypt webhook secret of %s/%s: %w",
					cfg.Repos[i].Owner, cfg.Repos[i].Name, err,
				)
			}

			cfg.Repos[i].WebhookSecret = &plaintext
		}
	}

	return &cfg, nil
}

func (c *Config) GetRepoConfig(owner, name string) *RepoConfig {
	for idx, _ := range c.Repos {
		if c.Repos[idx].Name == name && c.Repos[idx].Owner == owner {
			return &c.Repos[idx]
		}
	}
	return nil
}

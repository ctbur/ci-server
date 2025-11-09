package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	HostURL string        `toml:"host_url"`
	DataDir string        `toml:"data_dir"`
	GitHub  *GitHubConfig `toml:"github"`
	Repos   RepoConfigs   `toml:"repos"`
}

type GitHubConfig struct {
	AppID          uint64 `toml:"app_id"`
	InstallationID uint64 `toml:"installation_id"`
	PrivateKeyPath string `toml:"private_key_path"`
	// Name mapped to "encrypted_webhook_secret" - we decrypt it as part of loading the config
	WebhookSecret string `toml:"encrypted_webhook_secret,omitempty"`
}

type RepoConfigs []RepoConfig

type RepoConfig struct {
	Owner         string            `toml:"owner"`
	Name          string            `toml:"name"`
	DefaultBranch string            `toml:"default_branch"`
	EnvVars       map[string]string `toml:"env_vars"`
	BuildCmd      []string          `toml:"build_command"`
	// Name mapped to "encrypted_build_secrets" - we decrypt it as part of loading the config
	BuildSecrets map[string]string `toml:"encrypted_build_secrets"`
	DeployCmd    []string          `toml:"deploy_command"`
	// Name mapped to "encrypted_deploy_secrets" - we decrypt it as part of loading the config
	DeploySecrets map[string]string `toml:"encrypted_deploy_secrets"`
}

func Load(secretKey, configFile string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(configFile, &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// TODO: Validate that required fields are set

	// Decrypt webhook secret
	if cfg.GitHub != nil {
		plaintext, err := decryptSecret(secretKey, cfg.GitHub.WebhookSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
		}

		cfg.GitHub.WebhookSecret = plaintext
	}

	// Decrypt repo secrets
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

		for secretName := range cfg.Repos[i].DeploySecrets {
			plaintext, err := decryptSecret(secretKey, cfg.Repos[i].DeploySecrets[secretName])
			if err != nil {
				return nil, fmt.Errorf(
					"failed to decrypt deploy secret of %s/%s: %w",
					cfg.Repos[i].Owner, cfg.Repos[i].Name, err,
				)
			}

			cfg.Repos[i].DeploySecrets[secretName] = plaintext
		}
	}

	return &cfg, nil
}

func (r RepoConfigs) Get(owner, name string) *RepoConfig {
	for idx := range r {
		if r[idx].Name == name && r[idx].Owner == owner {
			return &r[idx]
		}
	}
	return nil
}

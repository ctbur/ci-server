package config

type Config struct {
	BuildDir string       `toml:"build_dir"`
	Repos    []RepoConfig `toml:"repos"`
}

func (c *Config) GetRepoConfig(owner, name string) *RepoConfig {
	for idx, _ := range c.Repos {
		if c.Repos[idx].Name == name && c.Repos[idx].Owner == owner {
			return &c.Repos[idx]
		}
	}
	return nil
}

type RepoConfig struct {
	Owner        string   `toml:"owner"`
	Name         string   `toml:"name"`
	BuildCommand []string `toml:"build_command"`
}

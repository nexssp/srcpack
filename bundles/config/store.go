package config

import (
	"os"
	"path/filepath"

	"github.com/nexssp/kernel/xerr"
	"github.com/nexssp/kernel/xfs"
	"gopkg.in/yaml.v3"
)

// UserConfig is the raw YAML structure of nexss.srcpack.yml as written
// by a user. It contains only fields the user actually wrote — no
// defaults, no built-in profiles. Used exclusively for read-modify-
// write operations; runtime consumers use Config.
type UserConfig struct {
	Prompts  map[string]string       `yaml:"prompts,omitempty"`
	Prompt   *FlexibleText           `yaml:"prompt,omitempty"`
	Options  *ConfigOptions          `yaml:"options,omitempty"`
	Editor   string                  `yaml:"editor,omitempty"`
	Dir      []DirEntry              `yaml:"dir,omitempty"`
	Targets  map[string]TargetGroup  `yaml:"targets,omitempty"`
	Profiles map[string]Profile      `yaml:"profiles,omitempty"`
	Editors  map[string]EditorFormat `yaml:"editors,omitempty"`
	Views    map[string][]string     `yaml:"views,omitempty"`
}

// ConfigStore reads and writes nexss.srcpack.yml. Every write goes
// through xfs.WriteFileAtomic so a crash mid-write never leaves a
// partial file.
type ConfigStore struct {
	Path string
}

func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{Path: path}
}

// LoadRaw reads the user config without overlaying built-in defaults.
// A missing file is not an error — it returns an empty config.
func (s *ConfigStore) LoadRaw() (UserConfig, error) {
	var cfg UserConfig
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, xerr.Internal("read config "+s.Path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, xerr.Validation("parse config "+s.Path, err)
	}
	return cfg, nil
}

// Mutate loads the raw config, applies fn, then atomically writes the
// result. A failed fn aborts the write. When dryRun is true the
// serialized YAML is returned without touching disk.
func (s *ConfigStore) Mutate(dryRun bool, fn func(*UserConfig) error) (string, error) {
	cfg, err := s.LoadRaw()
	if err != nil {
		return "", err
	}
	if mutateErr := fn(&cfg); mutateErr != nil {
		return "", mutateErr
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return "", xerr.Internal("marshal config", err)
	}
	if dryRun {
		return string(data), nil
	}
	if dir := filepath.Dir(s.Path); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return "", xerr.Internal("create config dir", mkErr)
		}
	}
	if err := xfs.WriteFileAtomic(s.Path, data, 0o600); err != nil {
		return "", xerr.Internal("write config "+s.Path, err)
	}
	return string(data), nil
}

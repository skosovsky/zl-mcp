package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"syscall"
)

type Config struct {
	StateDir   string `toml:"state_dir"`
	Collection struct {
		GroupIDs []string `toml:"group_ids"`
	} `toml:"collection"`
	Permissions struct {
		AllowJoin bool `toml:"allow_join"`
	} `toml:"permissions"`
	Storage struct {
		RetentionDays int `toml:"retention_days"`
	} `toml:"storage"`
	Logging struct {
		Level string `toml:"level"`
	} `toml:"logging"`
}

func Load(path string) (Config, error) {
	var c Config
	c.Storage.RetentionDays = 90
	c.Logging.Level = "info"
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("invalid config TOML: %w", err)
	}
	if c.StateDir == "" {
		return c, fmt.Errorf("state_dir is required")
	}
	if !filepath.IsAbs(c.StateDir) {
		c.StateDir = filepath.Join(filepath.Dir(path), c.StateDir)
	}
	c.StateDir, err = filepath.Abs(c.StateDir)
	if err != nil {
		return c, err
	}
	if c.Storage.RetentionDays < 0 {
		return c, fmt.Errorf("retention_days must be nonnegative")
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return c, fmt.Errorf("invalid logging.level")
	}
	seen := map[string]bool{}
	for _, id := range c.Collection.GroupIDs {
		if id == "" || seen[id] {
			return c, fmt.Errorf("group_ids must contain unique nonempty strings")
		}
		seen[id] = true
	}
	return c, nil
}
func (c Config) Allowed(id string) bool {
	for _, v := range c.Collection.GroupIDs {
		if v == id {
			return true
		}
	}
	return false
}
func (c Config) Prepare() error {
	if err := os.MkdirAll(c.StateDir, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(c.StateDir)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("state_dir must be a real directory")
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("state_dir must belong to current OS user")
	}
	return os.Chmod(c.StateDir, 0700)
}

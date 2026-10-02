package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

type Config struct {
	StateDir string `toml:"state_dir"`
	MCP      struct {
		Listen    string `toml:"listen"`
		TokenFile string `toml:"token_file"`
	} `toml:"mcp"`
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
		Level      string `toml:"level"`
		File       string `toml:"file"`
		MaxSizeMB  int    `toml:"max_size_mb"`
		MaxBackups int    `toml:"max_backups"`
	} `toml:"logging"`
}

func Load(path string) (Config, error) {
	var c Config
	c.Storage.RetentionDays = 90
	c.Logging.Level = "info"
	c.Logging.MaxSizeMB = 5
	c.Logging.MaxBackups = 3
	c.MCP.Listen = "127.0.0.1:18765"
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
	if c.MCP.TokenFile == "" {
		c.MCP.TokenFile = filepath.Join(c.StateDir, "mcp-token")
	}
	if !filepath.IsAbs(c.MCP.TokenFile) {
		c.MCP.TokenFile, err = filepath.Abs(filepath.Join(filepath.Dir(path), c.MCP.TokenFile))
		if err != nil {
			return c, err
		}
	}
	schema, err := contracts.Compile("service_config", "input")
	if err != nil {
		return c, err
	}
	if err = schema.Validate(map[string]any{"listen": c.MCP.Listen, "token_file": c.MCP.TokenFile}); err != nil {
		return c, fmt.Errorf("invalid MCP service configuration: %w", err)
	}
	host, port, err := net.SplitHostPort(c.MCP.Listen)
	ip := net.ParseIP(host)
	n, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("mcp.listen requires a literal loopback IP and port 1..65535")
	}
	if c.Logging.File == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return c, homeErr
		}
		c.Logging.File = filepath.Join(home, "Library", "Logs", "zl-mcp", "service.log")
	}
	if !filepath.IsAbs(c.Logging.File) {
		c.Logging.File, err = filepath.Abs(filepath.Join(filepath.Dir(path), c.Logging.File))
		if err != nil {
			return c, err
		}
	}
	logSchema, err := contracts.Compile("service_logging", "input")
	if err != nil {
		return c, err
	}
	if err = logSchema.Validate(map[string]any{"file": c.Logging.File, "max_size_mb": c.Logging.MaxSizeMB, "max_backups": c.Logging.MaxBackups}); err != nil {
		return c, fmt.Errorf("invalid service logging configuration: %w", err)
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

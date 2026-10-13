package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/pelletier/go-toml/v2"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type Config struct {
	Telemetry TelemetryConfig `toml:"telemetry"`
	StateDir  string          `toml:"state_dir"`
	MCP       struct {
		Listen    string `toml:"listen"`
		TokenFile string `toml:"token_file"`
	} `toml:"mcp"`
	Collection struct {
		GroupIDs      []string                 `toml:"group_ids"`
		Mode          string                   `toml:"mode"`
		Conversations []domain.ConversationRef `toml:"conversations"`
	} `toml:"collection"`
	Permissions struct {
		AllowJoin        bool     `toml:"allow_join"`
		AllowSend        bool     `toml:"allow_send"`
		SendRecipientIDs []string `toml:"send_recipient_ids"`
	} `toml:"permissions"`
	Storage struct {
		RetentionDays int `toml:"retention_days"`
	} `toml:"storage"`
	Logging struct {
		Level          string `toml:"level"`
		EventTraceFile string `toml:"event_trace_file"`
		File           string `toml:"file"`
		MaxSizeMB      int    `toml:"max_size_mb"`
		MaxBackups     int    `toml:"max_backups"`
	} `toml:"logging"`
}

func Load(path string) (Config, error) {
	var c Config
	c.Telemetry = c.Telemetry.WithDefaults()
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
		return c, errors.New("invalid config TOML")
	}
	if err = c.Telemetry.Validate(); err != nil {
		return c, err
	}
	if err = resolveMCPPaths(&c, path); err != nil {
		return c, err
	}
	if err = validateMCP(c); err != nil {
		return c, err
	}
	if err = configureLogging(&c, path); err != nil {
		return c, err
	}
	if err = validateRuntimeOptions(c); err != nil {
		return c, err
	}
	if err := c.validateCollection(); err != nil {
		return c, err
	}
	if err := c.validatePermissions(); err != nil {
		return c, err
	}
	return c, nil
}

func (c Config) validatePermissions() error {
	schema, err := contracts.Compile("service_permissions", "input")
	if err != nil {
		return err
	}
	ids := make([]any, 0, len(c.Permissions.SendRecipientIDs))
	for _, id := range c.Permissions.SendRecipientIDs {
		ids = append(ids, id)
	}
	if err = schema.Validate(
		map[string]any{
			"allow_join":         c.Permissions.AllowJoin,
			"allow_send":         c.Permissions.AllowSend,
			"send_recipient_ids": ids,
		},
	); err != nil {
		return fmt.Errorf("invalid sending permission configuration: %w", err)
	}
	return nil
}
func (c Config) Allowed(id string) bool {
	return c.Policy().Allows(domain.ConversationRef{Type: domain.ConversationGroup, ID: id})
}

func (c Config) Policy() domain.CollectionPolicy {
	p := domain.CollectionPolicy{All: c.Collection.Mode == "all", Selected: map[domain.ConversationRef]bool{}}
	for _, id := range c.Collection.GroupIDs {
		p.Selected[domain.ConversationRef{Type: domain.ConversationGroup, ID: id}] = true
	}
	for _, ref := range c.Collection.Conversations {
		p.Selected[ref] = true
	}
	return p
}

func (c Config) validateCollection() error {
	if c.Collection.Mode == "" {
		if c.Collection.Conversations != nil {
			return errors.New("collection.conversations requires an explicit mode")
		}
		return nil // Existing configs, including an empty group allowlist, keep their meaning.
	}
	if c.Collection.GroupIDs != nil {
		return errors.New("collection.mode cannot be combined with legacy group_ids")
	}
	args := map[string]any{"mode": c.Collection.Mode}
	if c.Collection.Conversations != nil {
		refs := make([]any, 0, len(c.Collection.Conversations))
		for _, r := range c.Collection.Conversations {
			refs = append(refs, map[string]any{"type": r.Type, "id": r.ID})
		}
		args["conversations"] = refs
	}
	s, err := contracts.Compile("collection_policy", "input")
	if err != nil {
		return err
	}
	if err = s.Validate(args); err != nil {
		return fmt.Errorf("invalid collection policy: %w", err)
	}
	return nil
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
		return errors.New("state_dir must be a real directory")
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	//nolint:gosec // G115: supported Unix targets use the same 32-bit uid_t for Getuid and Stat_t.Uid.
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return errors.New("state_dir must belong to current OS user")
	}
	//nolint:gosec // G302: 0700 is a private directory; owner execute is required for traversal.
	return os.Chmod(c.StateDir, 0700)
}

func resolveMCPPaths(c *Config, path string) error {
	var err error
	if c.StateDir == "" {
		return errors.New("state_dir is required")
	}
	if !filepath.IsAbs(c.StateDir) {
		c.StateDir = filepath.Join(filepath.Dir(path), c.StateDir)
	}
	c.StateDir, err = filepath.Abs(c.StateDir)
	if err != nil {
		return err
	}
	if c.MCP.TokenFile == "" {
		c.MCP.TokenFile = filepath.Join(c.StateDir, "mcp-token")
	}
	if !filepath.IsAbs(c.MCP.TokenFile) {
		c.MCP.TokenFile, err = filepath.Abs(filepath.Join(filepath.Dir(path), c.MCP.TokenFile))
		if err != nil {
			return err
		}
	}
	return nil
}

func validateMCP(c Config) error {
	schema, err := contracts.Compile("service_config", "input")
	if err != nil {
		return err
	}
	if err = schema.Validate(map[string]any{"listen": c.MCP.Listen, "token_file": c.MCP.TokenFile}); err != nil {
		return fmt.Errorf("invalid MCP service configuration: %w", err)
	}
	host, port, err := net.SplitHostPort(c.MCP.Listen)
	ip := net.ParseIP(host)
	n, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 {
		return errors.New("mcp.listen requires a literal loopback IP and port 1..65535")
	}
	return nil
}

func configureLogging(c *Config, path string) error {
	var err error
	if c.Logging.File == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return homeErr
		}
		c.Logging.File = filepath.Join(home, "Library", "Logs", "zl-mcp", "service.log")
	}
	if !filepath.IsAbs(c.Logging.File) {
		c.Logging.File, err = filepath.Abs(filepath.Join(filepath.Dir(path), c.Logging.File))
		if err != nil {
			return err
		}
	}
	logSchema, err := contracts.Compile("service_logging", "input")
	if err != nil {
		return err
	}
	if err = logSchema.Validate(
		map[string]any{
			"file":             c.Logging.File,
			"max_size_mb":      c.Logging.MaxSizeMB,
			"max_backups":      c.Logging.MaxBackups,
			"event_trace_file": c.Logging.EventTraceFile,
		},
	); err != nil {
		return fmt.Errorf("invalid service logging configuration: %w", err)
	}
	if c.Logging.EventTraceFile != "" &&
		(!filepath.IsAbs(c.Logging.EventTraceFile) || filepath.Clean(c.Logging.EventTraceFile) == filepath.Clean(c.Logging.File)) {
		return errors.New("event_trace_file must be absolute and separate from the service log")
	}
	return nil
}

func validateRuntimeOptions(c Config) error {
	if c.Storage.RetentionDays < 0 {
		return errors.New("retention_days must be nonnegative")
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("invalid logging.level")
	}
	seen := map[string]bool{}
	for _, id := range c.Collection.GroupIDs {
		if id == "" || seen[id] {
			return errors.New("group_ids must contain unique nonempty strings")
		}
		seen[id] = true
	}
	return nil
}

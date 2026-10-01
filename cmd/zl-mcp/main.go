package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("zl-mcp", flag.ContinueOnError)
	path := flags.String("config", "config.toml", "local TOML configuration")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() < 1 {
		return fmt.Errorf("usage: zl-mcp -config config.toml <login|collect|serve|approve-join preview_id>")
	}
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	if e = c.Prepare(); e != nil {
		return e
	}
	level := slog.LevelInfo
	switch c.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch flags.Arg(0) {
	case "collect":
		if flags.NArg() != 1 {
			return fmt.Errorf("collect takes no arguments")
		}
		return collector.Run(ctx, c)
	case "serve":
		if flags.NArg() != 1 {
			return fmt.Errorf("serve takes no arguments")
		}
		store, e := storage.Open(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Collection.GroupIDs, c.Storage.RetentionDays)
		if e != nil {
			return e
		}
		defer store.Close()
		server, e := mcpserver.New(store, c.StateDir)
		if e != nil {
			return e
		}
		return server.Run(ctx, &mcp.StdioTransport{})
	case "approve-join":
		if flags.NArg() != 2 {
			return fmt.Errorf("approve-join requires preview_id")
		}
		stat, e := os.Stdin.Stat()
		if e != nil || stat.Mode()&os.ModeCharDevice == 0 {
			return fmt.Errorf("approval requires an interactive local terminal")
		}
		client := control.New(c.StateDir)
		q, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		preview, e := client.Call(q, "cli_preview", map[string]any{"preview_id": flags.Arg(1)})
		if e != nil {
			return e
		}
		fmt.Fprintf(os.Stderr, "Join group %q (ID %s, moderator approval: %v)? Type JOIN to approve: ", preview["name"], preview["group_id"], preview["approval_required"])
		line, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil {
			return e
		}
		if strings.TrimSpace(line) != "JOIN" {
			return fmt.Errorf("approval cancelled")
		}
		approveCtx, approveCancel := context.WithTimeout(ctx, 5*time.Second)
		defer approveCancel()
		result, e := client.Call(approveCtx, "cli_approve", map[string]any{"preview_id": flags.Arg(1)})
		if e != nil {
			return e
		}
		fmt.Fprintln(os.Stdout, result["plan_token"])
		return nil
	case "login":
		if flags.NArg() != 1 {
			return fmt.Errorf("login takes no arguments")
		}
		unlock, e := local.Lock(c.StateDir)
		if e != nil {
			return e
		}
		defer unlock()
		loginCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		client, e := zalo.Login(loginCtx, c.StateDir, os.Stderr)
		if e != nil {
			return e
		}
		store, e := storage.Open(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Collection.GroupIDs, c.Storage.RetentionDays)
		if e != nil {
			return e
		}
		defer store.Close()
		if e = store.BindAccount(ctx, client.AccountID()); e != nil {
			return e
		}
		if e = client.Save(c.StateDir); e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, "Session saved locally. QR image removed.")
		return nil
	default:
		return fmt.Errorf("unknown command; available: login, collect, serve, approve-join")
	}
}

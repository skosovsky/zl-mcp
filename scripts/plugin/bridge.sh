#!/bin/sh
# STDIO bridge only. Never starts the collector or evaluates connection data.
set -eu
if [ -n "${ZL_MCP_BINARY:-}" ] || [ -n "${ZL_MCP_CONFIG:-}" ]; then
    binary=${ZL_MCP_BINARY:-}
    config=${ZL_MCP_CONFIG:-}
elif [ -n "${PLUGIN_DATA:-}" ] && [ -f "$PLUGIN_DATA/connection" ]; then
    exec 3< "$PLUGIN_DATA/connection"
    IFS= read -r binary <&3 || exit 1
    IFS= read -r config <&3 || exit 1
    if IFS= read -r extra <&3 || [ -n "${extra:-}" ]; then
        echo 'Zalo connection must contain exactly two lines.' >&2
        exit 1
    fi
    exec 3<&-
else
    echo 'Zalo is not configured. Run the setting-up-zalo skill; select an existing service.' >&2
    exit 1
fi
case "$binary" in /*) ;; *) echo 'Zalo binary must be an absolute path.' >&2; exit 1;; esac
case "$config" in /*) ;; *) echo 'Zalo config must be an absolute path.' >&2; exit 1;; esac
[ -x "$binary" ] && [ -f "$config" ] || { echo 'Zalo binary or configuration is unavailable.' >&2; exit 1; }
exec "$binary" -config "$config" serve

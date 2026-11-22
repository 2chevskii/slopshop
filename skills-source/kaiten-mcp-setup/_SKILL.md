---
name: kaiten-mcp-setup
description: Set up and troubleshoot the local Kaiten MCP server when the user wants to connect Kaiten or its tools are missing, failing to start, or unauthorized.
---

# Set up Kaiten MCP

## Check the client and credentials

1. Check whether the active client already has working Kaiten tools for the intended company. Reuse a working connection. When repairing a registration, preserve unrelated settings and other companies' connections.
2. Check that Node.js 24 or newer and `npx` are available in the environment that launches the server. Identify the company's HTTPS origin, such as `https://company.kaiten.ru`, without an API path, query, or fragment.
3. Have the user supply their Kaiten API token through the client's local credential mechanism. Keep the token out of chat, command arguments, tool output, and repository files. When inspecting existing settings, show only variable names and redact credential values.

## Register the server

Configure a user-level stdio server using `npx --yes @2chevskii/kaiten-mcp@2.0.0`. Set `KAITEN_ORIGIN` and provide either `KAITEN_TOKEN` or `KAITEN_TOKEN_FILE`. Use a distinct server name for each company. Apply configuration changes when the user asks for setup or repair. [Kaiten MCP setup](https://2chevskii.github.io/kaiten-mcp/guide)

For Codex, add or update the server in the active user `config.toml` (normally `~/.codex/config.toml`). Set the company origin and have the user make `KAITEN_TOKEN` available in the environment that launches Codex:

```toml
[mcp_servers.kaiten]
command = "npx"
args = ["--yes", "@2chevskii/kaiten-mcp@2.0.0"]
env_vars = ["KAITEN_TOKEN"]

[mcp_servers.kaiten.env]
KAITEN_ORIGIN = "https://company.kaiten.ru"
```

For file-based credentials, remove the `KAITEN_TOKEN` forwarding entry and set `KAITEN_TOKEN_FILE` in the server's `env` table to the absolute path of a private file containing the raw token. For other clients, use the same command and configure the environment using the client's user-level MCP settings. [Codex MCP settings](https://learn.chatgpt.com/docs/extend/mcp?surface=cli#configure-with-config-toml) · [Kaiten credential options](https://2chevskii.github.io/kaiten-mcp/connection-options)

## Verify the connection

Restart the client so it picks up the registration and environment. Once the tools appear, call `kaiten_navigation_read` with `get_current_user` and verify that the token belongs to the intended account. Use `list_spaces` if the user also wants to confirm access to a particular space.

For startup errors, check the client's MCP logs and confirm the Node.js version, `npx` path, origin, and credential source. For a 401 or 403, check the token's validity and permissions for the configured company. If the current session cannot reload its tools, tell the user how to verify in a fresh session. [Kaiten troubleshooting](https://2chevskii.github.io/kaiten-mcp/troubleshooting)

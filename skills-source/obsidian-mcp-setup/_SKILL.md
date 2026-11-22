---
name: obsidian-mcp-setup
description: Set up or troubleshoot the authenticated Streamable HTTP MCP connection to Obsidian's Local REST API with MCP community plugin. Use when the user asks to connect Obsidian or its MCP tools are missing, unauthorized, or unable to connect.
---

# Set up Obsidian MCP

## Find the endpoint and key

1. Check whether the client already has Obsidian MCP tools. If an existing connection works, reuse it. For a broken connection, preserve the existing server name, URL, and tool settings unless the user asks to change them.
2. Have the user enable **Local REST API with MCP** by Adam Coddington in Obsidian’s community plugins for the intended vault and keep Obsidian running. Confirm the MCP URL in the plugin’s settings. The default is `https://127.0.0.1:27124/mcp/`. For HTTPS, follow the upstream instructions to trust the plugin’s local CA in the client. Use `http://127.0.0.1:27123/mcp/` only if the user chooses loopback HTTP and enables the HTTP server in Obsidian. [Obsidian MCP setup](https://github.com/coddingtonbear/obsidian-local-rest-api#mcp-clients)
3. Have the user copy the API key from the Obsidian plugin settings directly into the client’s local credential mechanism. For an environment variable, have them set `OBSIDIAN_MCP_TOKEN` to the raw key in the environment that launches the client. Keep the key out of chat, command arguments, logs, and shared files. When inspecting an existing configuration, omit authorization header values from tool output.

## Register the server in the client

Use Streamable HTTP with `Authorization: Bearer <key>`. Use the active client’s supported secret or environment reference for the header. For Codex, a new registration can use the following command with the confirmed URL. [Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli#configure-with-config-toml)

```sh
codex mcp add obsidian --url https://127.0.0.1:27124/mcp/ --bearer-token-env-var OBSIDIAN_MCP_TOKEN
```

For an existing Codex registration that needs the credential source changed, set `bearer_token_env_var = "OBSIDIAN_MCP_TOKEN"` in the existing server table in `~/.codex/config.toml` and remove the old `Authorization` entry if present. Codex adds the `Bearer` prefix. Apply changes only when the user asks for setup or repair.

## Verify the connection

Reload the MCP configuration or restart the client so it sees the new server and environment. Verify that Obsidian’s tools appear. Read `obsidian://local-rest-api/status` if the client exposes MCP resources and check for `authenticated: true`. Otherwise, use a narrow read-only call supported by the installed server, such as `vault_list` for a user-selected directory. [Obsidian MCP resources](https://github.com/coddingtonbear/obsidian-local-rest-api#available-resources)

For a `401`, check that the key is current and available to the client process. For a certificate error, check whether the client trusts the local CA. For a refused connection, check that Obsidian is running and the chosen listener is enabled. Report what worked and any remaining step the user needs to perform.

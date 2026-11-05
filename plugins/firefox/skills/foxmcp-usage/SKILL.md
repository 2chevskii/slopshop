---
name: foxmcp-usage
description: Use FoxMCP to inspect and interact with Firefox tabs, pages, windows, bookmarks, and browser state.
---

# Firefox with FoxMCP

Before browser work, check whether FoxMCP tools are available. If they are missing or cannot connect, tell the user that FoxMCP and its Firefox extension must be installed and connected; point them to the [FoxMCP quick start](https://github.com/2chevskii/foxmcp#quick-start). The plugin expects the server at `http://localhost:3000/mcp/`. Do not claim the browser is accessible until a tool call succeeds.

When the server exposes `profiles_list`, call it first, select the intended profile, and use its `profile_id` for subsequent calls. Ask the user to disambiguate if needed. Profile and tab IDs are not portable between profiles and can change on reconnect. If the installed server exposes an older API, follow its actual tool schemas instead of inventing parameters.

Inspect the relevant tabs or page before acting. Use the server's tools and their returned schemas for navigation and interaction; prefer focused reads over broad browser data collection. Report the observed result and any failure.

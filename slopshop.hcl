marketplace {
  name         = "slopshop"
  display_name = "Slopshop"
}

plugin "firefox" {
  version     = "1.0.0"

  mcp_server "foxmcp" {
    type = "stdio"
    command = "uvx"
    args = ["--from", "foxmcp==1.7.0", "foxmcp", "--stdio", "--single-instance"]
  }
}

plugin "obsidian" {
  version     = "2.0.0"
  description = "Integrate coding agents with Obsidian vaults"

  skill "obsidian-mcp-setup" {
    path = "obsidian-mcp-setup"
  }
}

plugin "kaiten" {
  version     = "1.0.0"
  description = "Set up and troubleshoot Kaiten MCP"

  skill "kaiten-mcp-setup" {
    path = "kaiten-mcp-setup"
  }
}

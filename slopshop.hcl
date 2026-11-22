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

# plugin "kaiten" {
#   version     = "1.0.0"
#   description = "Work with Kaiten cards using Kaiten MCP and a workflow skill."

#   mcp_server "kaiten" {
#     type    = "stdio"
#     command = "npx"
#     args    = ["--yes", "@2chevskii/kaiten-mcp@latest"]
#   }

#   skill "kaiten-usage" {
#     path = "kaiten-usage"
#   }
# }

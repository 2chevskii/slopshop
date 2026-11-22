marketplace {
  name         = "slopshop"
  display_name = "Slopshop"
}

plugin "firefox" {
  version     = "0.1.0"
  description = "Control Firefox with FoxMCP and a browser workflow skill."

  mcp_server "foxmcp" {
    type = "http"
    url  = "http://localhost:3000/mcp/"
  }

  skill "foxmcp-usage" {
    path = "foxmcp-usage"
  }
}

plugin "kaiten" {
  version     = "1.0.0"
  description = "Work with Kaiten cards using Kaiten MCP and a workflow skill."

  mcp_server "kaiten" {
    type    = "stdio"
    command = "npx"
    args    = ["--yes", "@2chevskii/kaiten-mcp@latest"]
  }

  skill "kaiten-usage" {
    path = "kaiten-usage"
  }
}

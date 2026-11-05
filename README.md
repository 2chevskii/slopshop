# Slopshop

A small marketplace of plugins and skills for AI agents.

## Plugins

| Plugin | MCP server | Skill |
| --- | --- | --- |
| Firefox | [FoxMCP](https://github.com/2chevskii/foxmcp) | `foxmcp-usage` |
| Kaiten | [Kaiten MCP](https://github.com/2chevskii/kaiten-mcp) | `kaiten-usage` |

## Try locally

From a checkout, add this directory as a Codex marketplace:

```sh
codex plugin marketplace add .
codex plugin list
codex plugin add firefox@slopshop
codex plugin add kaiten@slopshop
```

Install the desired plugin using the plugin directory in ChatGPT desktop or the Codex CLI. The marketplace and plugin installation do not complete the external server setup below. See the [OpenAI plugin documentation](https://developers.openai.com/plugins/build/plugins) for the available client workflows. After publishing the repository, `codex plugin marketplace add 2chevskii/slopshop` can be used instead.

### Firefox

Install the Firefox extension and start FoxMCP as described in its [quick start](https://github.com/2chevskii/foxmcp#quick-start). The plugin connects to `http://localhost:3000/mcp/`; its HTTP server and extension must be running. The upstream README distinguishes its current source API from released versions: use matching server and extension versions.

### Kaiten

Kaiten MCP needs Node.js 24+, a Kaiten company origin and API token. The plugin starts `npx --yes @2chevskii/kaiten-mcp@latest` over stdio. Set `KAITEN_ORIGIN` and `KAITEN_TOKEN` securely in the environment inherited by the client before starting it, or configure the server separately using the [upstream setup guide](https://2chevskii.github.io/kaiten-mcp/guide). Do not commit tokens. If the client does not pass those variables to the plugin process, use its supported secret configuration or configure Kaiten MCP separately.

## Install skills separately

```sh
npx skills add . --list
npx skills add . --skill foxmcp-usage -a codex
```

After publication, replace `.` with `2chevskii/slopshop`. Installing a skill separately does not install or configure its MCP server; follow the corresponding upstream setup guide.

## Development

Install the lint tools with `npm ci`, then run `npm run lint` to check JSON and Markdown.

## License

Slopshop is licensed under [MIT](LICENSE). Upstream MCP servers and any upstream skills retain their own licenses; consult their respective repositories.

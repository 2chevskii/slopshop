# Slopshop

A small marketplace of plugins and skills for AI agents.

## Plugins

| Plugin  | MCP server                                            | Skill          |
| ------- | ----------------------------------------------------- | -------------- |
| Firefox | [FoxMCP](https://github.com/2chevskii/foxmcp)         | `foxmcp-usage` |
| Kaiten  | [Kaiten MCP](https://github.com/2chevskii/kaiten-mcp) | `kaiten-usage` |

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

Install Go 1.25 or later and Node.js 24 or later. From the repository root:

```sh
npm ci
npm run lint
go test ./...
npm run validate
npm run check
```

`slopshop.hcl` is the source for marketplace and plugin metadata. Run `npm run generate` to refresh skill copies and generated JSON, then commit them with the lockfile. `npm run check` compares the checkout against the manifest, local skill sources, and freshly fetched external skills. It leaves the checkout untouched. Generated JSON is excluded from Prettier because the generator owns its formatting; `check` validates its exact content instead.

The generator reports each file it will create, update, or remove and why. For JSON files, it also shows changed fields and their old and new values; `check` reports generated-file differences without applying them. Changes to skill copies include diffs for changed text files. Add `--verbose` (or `-v`) before or after any command for progress, including validation, unchanged files, and resolved import commits:

```sh
npm run generate -- --verbose
npm run check -- --verbose
npm run validate -- --verbose
npm run slopshop -- --help
```

When a plugin is removed from `slopshop.hcl`, the previous marketplace index identifies its generated `plugin.json` and `mcp.json` for cleanup. `check` reports the removals, and `generate` applies them.

## Manifest reference

`slopshop.hcl` uses native HCL. The root contains exactly one unlabeled `marketplace` block and zero or more `plugin "name"` blocks. Each plugin contains at least one `mcp_server "name"` or `skill "name"` block and may contain both. Use exactly one label on each `plugin`, `mcp_server`, and `skill` block. Attributes can appear once per block. Slopshop rejects unknown attributes and blocks, except for the additional attributes explicitly allowed in `mcp_server`.

Names use lowercase ASCII letters and digits, optionally separated by single hyphens. The pattern is `^[a-z0-9]+(?:-[a-z0-9]+)*$`. Plugin labels are unique across the manifest, MCP server labels are unique within their plugin, and skill labels are unique across the entire marketplace, including local and external skills.

Use quoted strings for text fields and static HCL expressions for values. Slopshop supplies no variables or functions. For fields declared as strings outside `mcp_server`, HCL also accepts values it can convert to strings, such as numbers and booleans. Use quoted values to make the intended text explicit.

### `marketplace`

| Attribute      | Type   | Required | Rules                                                  |
| -------------- | ------ | -------- | ------------------------------------------------------ |
| `name`         | String | Yes      | Marketplace identifier. Follows the name pattern.      |
| `display_name` | String | Yes      | Display name. Must contain a non-whitespace character. |

```hcl
marketplace {
  name         = "slopshop"
  display_name = "Slopshop"
}
```

### `plugin "name"`

| Attribute     | Type   | Required | Rules                                                                      |
| ------------- | ------ | -------- | -------------------------------------------------------------------------- |
| `version`     | String | Yes      | Plugin version in [Semantic Versioning 2.0.0](https://semver.org/) format. |
| `description` | String | No       | Plugin description. If provided, must contain a non-whitespace character.  |

```hcl
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
```

### `mcp_server "name"`

| Attribute | Type              | Applies to | Required | Rules                                                                                                                              |
| --------- | ----------------- | ---------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `type`    | String            | Both       | Yes      | Use `"stdio"` or `"http"`. Slopshop writes `"http"` as `"streamable-http"` in the generated JSON.                                  |
| `command` | String            | `stdio`    | Yes      | Executable to launch. Put arguments in `args`.                                                                                     |
| `args`    | List of strings   | `stdio`    | No       | Arguments passed to the executable.                                                                                                |
| `env`     | Map[string]string | `stdio`    | No       | Environment variables for the process, such as `env = { EXAMPLE = "value" }`. The client supplies `PLUGIN_ROOT` and `PLUGIN_DATA`. |
| `cwd`     | String            | `stdio`    | No       | Working directory. Defaults to the plugin root.                                                                                    |
| `url`     | String            | `http`     | Yes      | Absolute endpoint URL.                                                                                                             |
| `headers` | Map[string]string | `http`     | No       | Fixed request headers.                                                                                                             |

Slopshop validates the listed fields for the selected transport, rejects extra attributes, and writes the server entry to `plugins/<plugin>/mcp.json`. When setting `cwd`, use a path starting with `./`, or use `$${PLUGIN_ROOT}` or `$${PLUGIN_DATA}` with an optional path suffix. The doubled dollar sign preserves the placeholder through HCL evaluation. [Agent Plugins MCP configuration](https://agent-plugins.org/specification#721-discovery-and-configuration)

### `skill "name"`

| Attribute | Type   | Required | Rules                                                                                                                                                                  |
| --------- | ------ | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `path`    | String | Yes      | Clean relative directory path. For local skills, it is relative to `skills-source/`. For external skills, it is relative to the Git repository root.                   |
| `source`  | String | No       | Omit or use an empty string for local skills. For external skills, use an HTTPS or Git SSH repository URL.                                                             |
| `ref`     | String | No       | For external skills, use a branch, tag, or commit SHA. Omit it or use an empty string to resolve the remote default HEAD. For local skills, omit it or leave it empty. |

Use forward slashes in `path` and omit trailing slashes, redundant separators, and `.` or `..` segments. A standalone `.` selects the source root. Absolute paths, backslashes, colons, and NUL characters are invalid. Git SSH sources accept forms such as `git@example.com:owner/repository.git` and `ssh://git@example.com/owner/repository.git`. A nonempty `ref` cannot start with a hyphen or have leading or trailing whitespace.

### Validate the manifest

Run `npm run validate` or `go run ./tools/slopshop validate` from the repository root. The command exits with a nonzero status and prints errors with source locations where available. It reports syntax errors and checks any blocks HCL could recover, collecting independent schema and value errors in the same run.

Use `-o` or `--output` to select a diagnostic format:

| Format           | Output                                                                                                                                              |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rich` (default) | File, line, and column followed by a source excerpt with carets marking the error.                                                                  |
| `plain`          | One line per diagnostic, starting with its source location when available.                                                                          |
| `json`           | A JSON object on stdout with `valid` and a `diagnostics` array. Diagnostics include severity, summary, detail, and the source range when available. |

```sh
npm run validate -- --output json
```

Rich and plain diagnostics go to stderr. JSON mode keeps verbose progress on stderr so stdout can be parsed directly. JSON source ranges use one-based line and column numbers and zero-based byte offsets.

Validation reads only `slopshop.hcl`. Use `npm run check` to verify skill sources, the lockfile, and generated files.

## Skill sources

### Local skills

Place local skill sources under `skills-source/`. Name the main file `_SKILL.md` and declare the skill directory path relative to `skills-source/`:

```hcl
skill "foxmcp-usage" {
  path = "foxmcp-usage"
}
```

`generate` copies the complete directory to `plugins/<plugin>/skills/<name>/`, renaming the source `_SKILL.md` to `SKILL.md` in the copy. Edit the source, not the generated copy. If a skill is removed from the manifest, `generate` removes its generated directory; the source remains. `check` reports pending changes without applying them. Generated directories are recorded in the lock file, so untracked destinations and modified copies of removed skills are protected from deletion. The `name` and `description` fields in the main file's frontmatter are required, and its `name` must match the manifest. Skill names must be unique across the marketplace; extra entries in a plugin's `skills` directory are rejected. A skill that needs an MCP server should explain what to install or configure when the server is unavailable.

The [Vercel Skills CLI](https://github.com/vercel-labs/skills/blob/main/src/skills.ts) discovers `SKILL.md`, including with `--full-depth`, but does not treat `_SKILL.md` as a skill. The generator rejects files named `SKILL.md` anywhere under `skills-source/` (case-insensitively), including unlisted and nested sources, so they are not discovered accidentally. The generated `skills.sh.json` points to plugin skill directories.

### External skills

To include a skill from another Git repository, set `source` and `path`, and optionally `ref`.

```hcl
skill "example-skill" {
  source = "https://github.com/example/repository.git"
  path   = "path/to/skill/directory"
  ref    = "v1.2.3"
}
```

`generate` and `check` fetch every external skill declared in `slopshop.hcl`:

1. It clones each declared source, resolves `ref` (or the remote default HEAD) to a commit, and checks out that commit in a temporary directory. A commit SHA pins the selected revision.
2. It stages the directory at `path`, validates the skill, and hashes its files and directory structure. In the same run it stages local skills from `skills-source/`.
3. `generate` installs the skill copies, records the selected commits and content hashes in `slopshop.lock.json`, and regenerates the marketplace and plugin JSON. `check` reports any of those changes and exits nonzero when the checkout differs, including when a ref resolves to a new commit with unchanged skill contents.

Commit the copied files and lockfile along with any manifest changes. Both commands need Git access to declared external repositories. Use `--verbose` to see the resolved commits.

The importer rejects symlinks and refuses to overwrite a modified external skill. External skills retain their upstream `SKILL.md` names. Removing an external skill from the manifest and running `generate` removes its unmodified generated directory. To make an external skill local, copy its source into `skills-source/`, rename its main `SKILL.md` to `_SKILL.md`, remove `source` and `ref`, set the local `path`, and run `generate`.

Only files inside the referenced skill directory are copied. Check the upstream license and include any required license or attribution files when importing a skill; dependencies outside that directory also need to be handled explicitly.

## License

Slopshop is licensed under [MIT](LICENSE). Upstream MCP servers and any upstream skills retain their own licenses; consult their respective repositories.

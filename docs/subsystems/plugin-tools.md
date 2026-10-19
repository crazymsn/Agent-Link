# Plugin tools

A community plugin (an OpenCode plugin, an npm package in the plugin host) can give tools: OpenCode's `tool` hook, `{tool: {name: tool({description, args, execute})}}`. In OpenCode only OpenCode's own model can call them. Agent Link offers them to every agent instead, as an MCP server of its own at `/mcp/Agent Link/tools` (Streamable HTTP), which any agent that takes an MCP server can be given. It is the gateway's tool point (P4 of [Plugin runtime](plugin-runtime.md)) opened to community plugins. The built-in plugin `tools` serves it. The provider plugins in the same host are [Provider and plugin ownership](provider-plugins.md); the gateway middleware is [Gateway middleware](gateway-middleware.md).

## Responsibilities and sources of truth

| Part | Responsibility | Source |
| --- | --- | --- |
| Collect | Each loaded plugin's tools: the names that match `^[A-Za-z0-9_-]{1,48}$` and have an `execute`, the last of a name winning, as in OpenCode. Each tool's `args` becomes JSON Schema: a zod shape (v3 or v4, `.describe`, `.optional`, `.default`, enums, arrays, nested objects) or a zod object, while a plain JSON Schema object is kept as it is. They are returned with the plugin list (`Loaded.Tools`). | `toolsOf`, `jsonSchema`, `argsSchema`, `toolList` in [`host.js`](../../internal/plugin/host.js); `Tools` in [`tools.go`](../../internal/plugin/tools.go) |
| Run | The host's `tool` call: it checks the arguments against the tool's own zod schema (defaults filled in; a wrong argument is an error naming its path), then races `execute(args, ctx)` against `ToolTimeout` (55s, under the 60s an MCP client gives a call). `ctx` is OpenCode's: `agent` is the calling agent as the gateway knows it, `abort` fires at the timeout, and `directory` is the host's. A string answer is the text, `{output}` its output, and anything else is its JSON. | `parsedArgs`, `runTool` in `host.js`; `RunTool` in `tools.go` |
| Note | After every load the host notes which plugins gave which tools in `plugin-tools.json` (settings dir), written whole and only when it changed. `ToolsOn` counts the plugins that are on in plugins.json and have tools there. A note that can't be read or parsed is an error, so the plugin stays on. `ToolNames` is the note for a Plugins page whose host isn't running. | `noteTools`, `ToolsOn`, `ToolNames` in `tools.go` |
| Serve | `tools/list` names each tool `<plugin>_<tool>`: the plugin's short name, as its Plugins row names it (`plugin.ShortName`, which takes off the scope and one leading `middleware-`, `opencode-`, `tools-`, `page-` or `agent-`: opencode-weather-tools → weather-tools, @Agent Link-community/tools-calc → calc; a plugin added from a folder, the folder's name), at most 24 characters, in `[A-Za-z0-9_-]`, then the tool. Each name is at most 64 characters, and a name two would share gets `_2`, `_3`. `tools/call` runs one; an unknown name is error -32602. It is guarded as the search MCP server is (`serveMCP`: a web page's request is refused, a remote one needs a key), with a 4 MiB body. | [`tools_mcp.go`](../../internal/gateway/tools_mcp.go) |
| Gate | `plugins/tools` ("Plugin tools", requires `gateway`, Essential) serves `gwtool.PluginTools`. `InUse` is `plugin.ToolsOn`, so it is on while a plugin that is on gives a tool. The gateway's `KeepFresh` watches `plugin-tools.json` too. | [`plugins/tools`](../../plugins/tools/tools.go), [`gwtool.go`](../../internal/gwtool/gwtool.go), [`gateway.go`](../../internal/gateway/gateway.go) |
| GUI | Each Installed row of a plugin that is on and gives tools lists them by their MCP names, each with its description on hover, plus a button that copies the server's address (`mcpAddress`). A plugin of tools alone wears the Tools chip, not Provider. Discover lists `kind: "tools"` market entries under Tools for agents. | `pluginEntryJSON.Tools` in [`plugins.go`](../../internal/gui/plugins.go); `toolsLine`, `kindOf` and `KINDS.tl` in `plugins.js` |
| CLI | `Agent Link plugin` lists a plugin's tools by the same MCP names, and a plugin's page by its title and what it may read. The host loads no plugin that is off, so one lists no tools. | `toolsLine`, `pageLine` in [`plugins_cli.go`](../../internal/cli/plugins_cli.go) |
| User docs | How to write a tool and add the address to an agent | [useAgent Link.ai/docs/plugins#tools](https://useAgent Link.ai/docs/plugins#tools) (`site/public/docs/*plugins.html`) |

plugins.json is the source of truth for which plugins are on. The host only loads those, so only their tools are listed.

## Runtime path

1. An agent's MCP client posts `tools/list` to `/mcp/Agent Link/tools`. `servedTools` returns nothing while the point isn't served or no plugin is on; then the plugin host (Bun) isn't started for it. Otherwise it asks the host for its plugins' tools (`plugin.Tools`, which starts the host if need be) and names them (`namedTools`).
2. `tools/call` finds the tool by its MCP name in the same list. While the point is off it answers `isError` "Agent Link's Plugin tools plugin is off…", without asking the host.
3. `toolsMCPRun` holds the call to the calling gateway key's limit through `reserveFor`, the same way in as the chat APIs and the search MCP server ([Plugin runtime](plugin-runtime.md), budget point). A refusal, or a limited key with the Key limits plugin off, is an `isError` answer, and the tool isn't run.
4. `plugin.RunTool` asks the host to run it. The answer, or the error (`<tool>'s arguments are wrong: …`, `<tool> took longer than 55s`, or what the tool threw), goes back to the agent as the call's text, with `isError` set on an error.

A tool's answer goes back to the agent that called it, on this computer or on a shared one with a key. It never goes to a vendor, so masking has nothing to do with it. Whatever the agent sends a model next is masked as any request is.

## Constraints and failure behavior

- Off means off: no tool is listed, a call says the plugin is off, and the host is not asked (`TestToolsMCPOff`).
- A plugin's tool runs in the plugin host with the user's rights, as its provider code does. Installing a plugin is trusting it.
- One host serves every plugin. A tool that never answers is answered at `ToolTimeout`, and its call's slot in the host is left for its own abort to end; the host goes on serving (`TestPluginTools`).
- Names are stable for a given set of plugins on. Turning on a plugin earlier in plugins.json that gives a tool of the same MCP name renumbers the later one.
- Wiring is the user's: the Plugins page copies the address, and Agent Link writes no agent's MCP config for it. There is no Library entry yet.

## Verification

```sh
go test ./internal/plugin/ -run Tool
go test ./internal/gateway/ -run 'ToolsMCP|ServingGates|OwnMCPRejects' -timeout 45m
go test ./plugins/tools/ ./internal/pluginrt/
go test ./internal/gui/ -run 'Builtin|Plugins'
node --test internal/gui/tests/plugin-tools.test.cjs internal/gui/tests/builtins.test.cjs
```

The example plugin the tests load is [`internal/plugin/testdata/tools`](../../internal/plugin/testdata/tools/index.js): tools with a zod shape, a raw JSON Schema, one that sleeps past the timeout, one that throws, and two that are left out.

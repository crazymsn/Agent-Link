# Agent Link DeepCloud Rebrand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebase the project on upstream `ccb2eea1597d6d68e18427d6a9a2cbbe437c0602`, rebrand the user-facing application and plugin system as Agent Link / DeepCloud, remove editable custom providers in favor of one fixed API distribution provider, set the new gateway default port, remove community/version settings surfaces, rewrite documentation, and publish the verified result to `crazymsn/Agent-Link`.

**Architecture:** Keep legacy storage keys, environment variables, provider IDs, and paths only where they are required to read existing user data or preserve agent compatibility; introduce new display names and new branding assets at the UI/build boundary. Enforce the fixed provider policy in Go storage/API code first, then make the GUI reflect that policy. Use a migration that removes old user-defined custom providers and creates/maintains the fixed API distribution provider with a stable internal ID, stable URL, and DeepCloud icon.

**Tech Stack:** Go 1.26.8, Wails v3, embedded HTML/CSS/JS GUI, Playwright GUI tests, Windows resource JSON, macOS AppIcon/Info.plist, GitHub CLI authentication.

---

## File Map

- **Brand and build identity:** `go.mod`, `main.go`, `build/windows/winres.json`, `build/darwin/Info.plist`, `build/darwin/AppIcon.icon/**`, `build/windows/icon-*.png`, `build/icon/*`, `internal/gui/assets/index.html`, `internal/gui/assets/icons/**`, `internal/gui/assets/app.js`, `internal/gui/assets/i18n.js`, `internal/gui/assets/*.css`, `site/public/**`, `.github/workflows/**`.
- **Provider policy and migration:** `internal/provider/provider.go`, `internal/provider/provider_order.go`, `internal/provider/migrate.go`, `internal/provider/provider_test.go`, `internal/provider/provider_order_test.go`, new `internal/provider/agentlink.go` and `internal/provider/agentlink_test.go`.
- **GUI provider API:** `internal/gui/providers.go`, `internal/gui/api.go`, `internal/gui/providers_*_test.go`, new fixed-provider API tests as needed.
- **Settings and gateway default:** `internal/settings/settings.go`, `internal/settings/settings_test.go`, `internal/gui/app.js`, `internal/gui/port_test.go`, related settings API tests.
- **Settings surface removal:** `internal/gui/assets/app.js`, `internal/gui/assets/i18n.js`, `internal/gui/api.go`, `internal/gui/*settings*_test.go`, and the owning GUI subsystem reference if a documented route/contract changes.
- **Documentation and legal:** `README.md`, `README.zh-CN.md`, `LICENSE` retained, `AGENTS.md` only where it describes the product brand, `docs/**` and `site/public/docs/**` where public product text names the old project. Upstream MIT license and required notices remain.
- **Brand asset:** copy the supplied DeepCloud image into a source asset file under `build/brand/deepcloud-source.jpg`; generate deterministic PNG/ICO/ICNS variants using the existing build tooling or checked-in generated outputs.

## Task 1: Establish the latest Agent Link worktree

**Files:**
- Source: `D:\项目工程\Agent Link-latest` at commit `ccb2eea1597d6d68e18427d6a9a2cbbe437c0602`
- Worktree: create a dedicated Git worktree/branch from the latest `main` before edits.

- [ ] **Step 1: Verify the exact upstream base.**

Run:

```powershell
git -C D:\项目工程\Agent Link-latest rev-parse HEAD
git -C D:\项目工程\Agent Link-latest status --short --branch
```

Expected: the SHA is `ccb2eea1597d6d68e18427d6a9a2cbbe437c0602` and the tree is clean.

- [ ] **Step 2: Create an isolated working branch.**

Run:

```powershell
git -C D:\项目工程\Agent Link-latest switch -c agent-link-deepcloud
```

Expected: a new branch named `agent-link-deepcloud`.

- [ ] **Step 3: Record the baseline before edits.**

Run:

```powershell
git -C D:\项目工程\Agent Link-latest diff --stat
```

Expected: no output.

## Task 2: Add the DeepCloud brand asset and replace desktop identity

**Files:**
- Create: `build/brand/deepcloud-source.jpg`
- Create/modify: `build/brand/deepcloud.svg`, `build/brand/deepcloud.png`
- Modify: `build/windows/winres.json`, `build/windows/icon-16.png`, `build/windows/icon-32.png`, `build/windows/icon-48.png`, `build/windows/icon-64.png`, `build/windows/icon-128.png`, `build/windows/icon-256.png`
- Modify: `build/darwin/Info.plist`, `build/darwin/AppIcon.icon/**`, `build/darwin/Agent Link.icns`
- Modify: `build/icon/gen.go`, `build/icon/Agent Link.svg`, `build/icon/Agent Link-small.svg` only if the existing generator must be adapted to the new source
- Modify: `internal/gui/assets/index.html`, `internal/gui/assets/app.css`, GUI icon assets

- [ ] **Step 1: Add a deterministic asset conversion check.**

Create a small build-time test or generator assertion that the source brand image exists, has a non-zero raster size, and produces all required Windows/macOS/GUI outputs. The assertion must fail if one generated output is missing.

- [ ] **Step 2: Run the asset check before implementation.**

Run:

```powershell
go test ./build/brand ./build/icon
```

Expected: FAIL because the new brand package/output contract does not exist yet.

- [ ] **Step 3: Implement the DeepCloud asset pipeline.**

Use the supplied image at `C:\Users\Alex\AppData\Local\Temp\orca-paste-1792336461462-0fc42ad6-a10d-48e2-90c1-c2afbb29a4f8.png` as input, copy it into `build/brand/deepcloud-source.jpg`, and generate a transparent square icon plus full lockup variants. Keep generated files deterministic and checked in so packaging does not depend on a local temp path.

- [ ] **Step 4: Replace Windows identity.**

Set `CompanyName`, `FileDescription`, and `ProductName` to `Agent Link`, set the company text to `DeepCloud`, set `OriginalFilename` to `agent-link.exe`, change the manifest identity to a stable Agent Link identifier, and point the resource group to the generated DeepCloud icons.

- [ ] **Step 5: Replace macOS identity.**

Set `CFBundleName`, `CFBundleDisplayName`, `CFBundleExecutable`, `CFBundleIconFile`, `CFBundleIdentifier`, URL scheme, and human-readable copyright to the Agent Link / DeepCloud values. Update the AppIcon assets and generated ICNS output.

- [ ] **Step 6: Run build asset checks.**

Run:

```powershell
gofmt -w build/brand/*.go build/icon/*.go
go test ./build/brand ./build/icon
```

Expected: PASS and all generated icon files exist.

- [ ] **Step 7: Commit the asset boundary.**

```powershell
git add build/brand build/windows build/darwin build/icon internal/gui/assets/index.html internal/gui/assets/app.css
git commit -m "brand: ship Agent Link DeepCloud desktop identity"
```

## Task 3: Introduce the fixed API distribution provider and remove old custom providers

**Files:**
- Create: `internal/provider/agentlink.go`
- Create: `internal/provider/agentlink_test.go`
- Modify: `internal/provider/provider.go`, `internal/provider/provider_order.go`, `internal/provider/migrate.go`
- Modify: `internal/provider/provider_test.go`, `internal/provider/provider_order_test.go`
- Modify: `internal/gui/providers.go`, `internal/gui/api.go`
- Create/modify: `internal/gui/provider_agentlink_test.go`
- Modify: `internal/gui/assets/app.js`, `internal/gui/assets/i18n.js`, `internal/gui/assets/icons/**`

- [ ] **Step 1: Write failing provider policy tests.**

Add tests that assert:

```go
func TestAgentLinkProviderIsFixed(t *testing.T) {
    p := AgentLinkProvider()
    if p.Name != "API 分发站" || p.Chat != "https://api.meding.site/v1" || p.Icon != "deepcloud" {
        t.Fatalf("unexpected fixed provider: %#v", p)
    }
    if !p.Fixed || p.ID == "" {
        t.Fatalf("provider is not fixed: %#v", p)
    }
}

func TestAgentLinkProviderPrecedesClaude(t *testing.T) {
    got := ordered([]Provider{{ID: "claude"}, {ID: AgentLinkProviderID}}, nil)
    if got[0].ID != AgentLinkProviderID {
        t.Fatalf("first provider = %q, want %q", got[0].ID, AgentLinkProviderID)
    }
}

func TestRemoveCustomProvidersLeavesBuiltinsAndAgentLink(t *testing.T) {
    // Store one arbitrary user provider and one built-in subscription, run the
    // migration, then assert the user provider is gone and Agent Link remains.
}
```

Use the existing test sandbox/store helpers; do not read or write a live `~/.config/Agent Link` directory.

- [ ] **Step 2: Run the new tests before implementation.**

Run:

```powershell
go test ./internal/provider -run 'TestAgentLinkProvider|TestRemoveCustomProviders' -count=1
```

Expected: FAIL because the fixed provider type, policy flag, migration, and ordering rule are absent.

- [ ] **Step 3: Add the fixed provider definition.**

Define one stable internal ID, for example `agent-link-api`, and a constructor returning:

```go
Provider{
    ID: AgentLinkProviderID,
    Name: "API 分发站",
    Icon: "deepcloud",
    Chat: "https://api.meding.site/v1",
    KeyName: "API 分发站",
    Fixed: true,
}
```

Extend the provider data model with a serialized/internal fixed marker only if the current model requires it; otherwise derive fixed status from the stable ID so a user cannot make an arbitrary provider fixed by editing JSON.

- [ ] **Step 4: Enforce fixed-provider invariants in storage and API.**

Ensure `All()` always includes the fixed provider, `Save()` rejects changes to its URL/name/icon and rejects new arbitrary custom provider records, `Delete()` rejects deleting the fixed provider, and order calculation places the fixed ID before Claude and every other provider regardless of a stale saved order.

- [ ] **Step 5: Add the migration that deletes old custom providers.**

During the existing provider migration/startup path, classify user-defined providers separately from subscriptions/presets/plugins, delete only those custom records, remove their IDs from saved order, then insert or repair the fixed provider. Make the operation idempotent and preserve built-in subscription accounts.

- [ ] **Step 6: Update GUI provider payloads and actions.**

Mark the fixed provider as non-editable/non-deletable in the JSON payload and remove the add-custom flow. Keep preset/subscription tiles that are part of the built-in product, but any request attempting to create/edit/delete an arbitrary custom provider must receive a clear 4xx error. Render the fixed provider first with the DeepCloud icon and a key input only.

- [ ] **Step 7: Run provider tests.**

Run:

```powershell
gofmt -w internal/provider/agentlink.go internal/provider/agentlink_test.go internal/provider/provider.go internal/provider/provider_order.go internal/provider/migrate.go internal/gui/providers.go internal/gui/api.go internal/gui/provider_agentlink_test.go
go test ./internal/provider ./internal/gui -run 'AgentLink|ProviderOrder|ProviderSave|ProviderList' -count=1
```

Expected: PASS, with old custom-provider deletion and fixed ordering covered.

- [ ] **Step 8: Commit the provider policy.**

```powershell
git add internal/provider internal/gui/providers.go internal/gui/api.go internal/gui/provider_agentlink_test.go internal/gui/assets/app.js internal/gui/assets/i18n.js internal/gui/assets/icons
git commit -m "provider: add fixed Agent Link API distribution"
```

## Task 4: Change the gateway default port to 7500 without overriding user settings

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`
- Modify: `internal/gui/port_test.go`
- Modify: `internal/gui/assets/app.js`, `internal/gui/assets/i18n.js` only if the displayed default is hard-coded

- [ ] **Step 1: Add the failing default-port test.**

Add a test that loads an empty settings file and asserts `DefaultPort == 7500` and `GatewayAddr()` ends in `:7500`; add a second test with `Settings{Port: 4010}` and assert it remains `:4010`.

- [ ] **Step 2: Run it before implementation.**

```powershell
go test ./internal/settings ./internal/gui -run 'DefaultPort|GatewayAddr|Port' -count=1
```

Expected: FAIL because the current default is 3425.

- [ ] **Step 3: Change only the default constant and derived display.**

Set `DefaultPort = 7500`. Keep `Settings.Port` semantics unchanged so a saved non-zero port remains authoritative. Do not rewrite an existing settings file just because the default changed.

- [ ] **Step 4: Run the focused tests.**

```powershell
go test ./internal/settings ./internal/gui -run 'DefaultPort|GatewayAddr|Port' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the port change.**

```powershell
git add internal/settings/settings.go internal/settings/settings_test.go internal/gui/port_test.go internal/gui/assets/app.js internal/gui/assets/i18n.js
git commit -m "gateway: default new installs to port 7500"
```

## Task 5: Remove Community and Version surfaces from Settings

**Files:**
- Modify: `internal/gui/assets/app.js`
- Modify: `internal/gui/assets/i18n.js`
- Modify: `internal/gui/api.go`
- Modify/delete: settings/update/community-specific GUI tests identified by search
- Modify: `docs/subsystems/gui-shell.md` if the documented settings contract changes

- [ ] **Step 1: Add failing GUI/API assertions.**

Add or update tests so the settings payload contains no community route/action and no version/update row/action, while normal theme/language/network settings remain. Assert that direct calls to removed settings actions return 404 or an explicit unsupported response.

- [ ] **Step 2: Run the tests before implementation.**

```powershell
go test ./internal/gui -run 'Settings|Update|Community|Version' -count=1
```

Expected: FAIL because the current app still exposes update/version and community paths.

- [ ] **Step 3: Remove the settings UI and routes.**

Delete the nav entries, render branches, click handlers, update/version API calls, community links, and their translation entries. Preserve agent update actions that are part of an agent page only if they are unrelated to the removed Settings version surface; verify the user-visible Settings page has no version/community controls.

- [ ] **Step 4: Remove dead backend handlers/imports.**

Delete only handlers that exist solely for the removed Settings surfaces. Keep shared update/download code if any non-Settings path still uses it; use compiler errors and references to decide rather than deleting by name.

- [ ] **Step 5: Run GUI tests in both engines and languages.**

Run the repository’s full UI command:

```powershell
make test-ui
BROWSER=webkit node --test internal/gui/tests/*.test.js
```

Run the affected Settings tests with English, Chinese, `gui-zh-tw`, `gui-ja`, and `gui-de` fixtures where the repository provides them. Expected: no Settings/community/version regressions.

- [ ] **Step 6: Commit the Settings cleanup.**

```powershell
git add internal/gui docs/subsystems/gui-shell.md
 git commit -m "gui: remove community and version settings surfaces"
```

## Task 6: Rebrand application, plugins, site, and repository references

**Files:**
- Modify: `go.mod`, all non-test user-facing Go comments/strings and package labels that expose the old product name
- Modify: `internal/gui/assets/app.js`, `internal/gui/assets/i18n.js`, `internal/gui/assets/index.html`, `internal/gui/assets/*.css`
- Modify: `internal/plugin/**`, `plugins/**`, plugin templates/manifests/docs
- Modify: `README.md`, `README.zh-CN.md`, `site/public/**`, `.github/workflows/**`, `Dockerfile`, `Makefile`, `main.go`
- Preserve: legacy environment variable names, provider IDs, config paths, migration IDs, and fixtures when compatibility requires them; add comments explaining compatibility where needed.

- [ ] **Step 1: Generate a categorized old-brand inventory.**

Run:

```powershell
rg -n -i --glob '!**/.git/**' --glob '!**/node_modules/**' 'Agent Link|yetone/Agent Link|Agent Link-community' . > brand-inventory.txt
```

Classify each match as `display`, `build`, `plugin-template`, `docs`, `compatibility`, or `test-fixture`. Do not modify compatibility values blindly.

- [ ] **Step 2: Add a guard test for user-visible branding.**

Create a test that checks built GUI HTML/JS, desktop manifests, plugin templates, README, and public site entry points for the old user-visible brand and target repository. The allow-list must contain only legacy compatibility keys, migration fixtures, and retained MIT/legal text.

- [ ] **Step 3: Replace user-visible and build-time names.**

Set package/module display metadata, window titles, tray labels, GUI errors, plugin host labels, plugin template names, site titles, Docker labels, Make targets, workflow URLs, and repository links to Agent Link / DeepCloud / `https://github.com/crazymsn/Agent-Link`. Use `t()` for every GUI string and update all language maps with matching placeholders.

- [ ] **Step 4: Update plugin system branding without breaking protocol IDs.**

Change plugin host display names, plugin documentation, examples, and new plugin templates to Agent Link. Keep provider IDs and serialized plugin IDs stable for existing installed plugins; document that compatibility IDs are protocol identifiers, not branding.

- [ ] **Step 5: Rewrite README files.**

Write English and Simplified Chinese READMEs around Agent Link / DeepCloud. Include the API distribution URL `https://api.meding.site`, explain that users create an API token there and enter it in the fixed `API 分发站` subscription provider to access all models, document the default gateway port 7500, and link only to `https://github.com/crazymsn/Agent-Link`. Keep the MIT license reference and attribution required by the source license.

- [ ] **Step 6: Run the brand guard and documentation checks.**

```powershell
go test ./internal/gui ./internal/provider ./internal/plugin -run 'Brand|Plugin|Provider' -count=1
rg -n -i --glob '!**/.git/**' 'github_pat_|ghp_|github_token|yetone/Agent Link|Agent Link-community' .
```

Expected: no secrets; only explicitly documented compatibility/legal matches remain.

- [ ] **Step 7: Commit the rebrand.**

```powershell
git add go.mod main.go Dockerfile Makefile internal plugins site .github README.md README.zh-CN.md
 git commit -m "brand: rebrand application and plugins as Agent Link"
```

## Task 7: Full verification and push to the target repository

**Files:**
- No source changes unless a verification failure identifies a real defect.
- Generated verification logs: keep outside the repository or under a session-owned temp directory.

- [ ] **Step 1: Check the final diff and secrets.**

```powershell
git status --short
git diff --check
git grep -n -i -E 'github_pat_|ghp_|github_token|api[_-]?key[[:space:]]*[:=][[:space:]]*[^$<]' -- ':!README.md' ':!README.zh-CN.md'
```

Expected: no pasted PAT or other credential. The fixed API URL is public and may appear in source/docs.

- [ ] **Step 2: Run formatting and static checks.**

```powershell
gofmt -l $(git diff --name-only -- '*.go')
go vet ./...
$env:GOOS='linux'; go vet -tags nogui ./...; Remove-Item Env:GOOS
$env:GOOS='windows'; go vet ./...; Remove-Item Env:GOOS
```

Expected: `gofmt -l` prints nothing and all vet commands pass.

- [ ] **Step 3: Build all supported targets.**

```powershell
go build ./...
$env:GOOS='linux'; go build -tags nogui ./...; Remove-Item Env:GOOS
$env:GOOS='windows'; go build ./...; Remove-Item Env:GOOS
```

Expected: all builds pass.

- [ ] **Step 4: Run the full Go suite in a sandbox.**

Use a temp HOME and the repository’s documented pinned `GOPATH`, `GOMODCACHE`, and `GOCACHE` setup, then run:

```powershell
go test -timeout 45m ./...
```

Expected: every package passes; record any package not run and why.

- [ ] **Step 5: Run the full GUI suite.**

Run Chromium and WebKit UI suites at narrow widths and all repository locales. Confirm Settings has no Community/Version surfaces, API 分发站 is first and fixed, and the Agent Link / DeepCloud logo appears in the window/tray/provider card.

- [ ] **Step 6: Perform a real sandbox smoke test.**

In a disposable HOME, launch the built desktop/gateway binary, confirm the first-run gateway address is `127.0.0.1:7500`, add an API token only through the fixed API 分发站 provider, request `/v1/models` against a controlled local stub or the authorized API endpoint, and verify the provider cannot be edited/deleted and no custom provider can be added. Do not use the live user configuration.

- [ ] **Step 7: Prepare the target remote without embedding credentials.**

```powershell
git remote remove origin
git remote add origin https://github.com/crazymsn/Agent-Link.git
git remote -v
```

Use the already authenticated `gh` credential helper or a credential prompt; never put the PAT in the URL or a file.

- [ ] **Step 8: Push only after verification is green.**

```powershell
git push -u origin agent-link-deepcloud:main
```

Expected: the target repository’s `main` advances to the reviewed commit. Verify:

```powershell
gh api repos/crazymsn/Agent-Link/commits/main --jq '.sha'
gh repo view crazymsn/Agent-Link --json url,defaultBranchRef,description
```

- [ ] **Step 9: Record the reviewed head and checks.**

Create a final commit body or release note stating the exact reviewed SHA, commands run, GUI engines/locales tested, real smoke test status, and any checks not run. Do not claim a test or real vendor call that was blocked or unavailable.

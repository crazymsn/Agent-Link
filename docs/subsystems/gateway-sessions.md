# Gateway Session Aggregation

The Sessions API combines file-backed native sessions with a read-only
projection of gateway usage records. A gateway session identity is exactly
`AgentOf(record.Agent) + record.Session`, where the session is the one the
client sent (`X-Agent Link-Session`, or its own session header), whatever it starts
with. The response returns that id in `X-Agent Link-Session`; listed browser
origins can read this header through CORS. agentlink never makes up an id for a
request without one, so such a request has no session and its text is not
recorded: text kept under an id nothing lists couldn't be opened. Prompt text,
caller keys and network addresses are never used to guess conversation ownership.
So the configs agentlink writes have each agent name its session: Pi's provider
block sets `compat.sendSessionAffinityHeaders` (`x-session-affinity` on Chat and
Messages; Responses sends `session_id` anyway), and omp 16.0.6 and later gets
`compat.promptCacheSessionHeader: x-session-affinity`. A user's own
`sendSessionAffinityHeaders` is kept (`piBlockKept`). Verification:
`TestPiNamesItsSessionToTheGateway`, `TestOmpNamesItsSessionToTheGateway`.
Older ledger records without `Session` remain excluded. The projection is
computed from the usage ledger and is not persisted. Window projections and
routing summaries reuse versioned log snapshots; appends and rewrites invalidate
them, and price/provider changes also invalidate priced projections. Single-session
lookups use the cached identity index. Native exclusions are applied per caller.

WorkBuddy and CodeBuddy Code name their conversation in `X-Conversation-ID`.
The gateway reads it as a native session header, after the existing native
headers; `X-Agent Link-Session` still overrides the recorded session without
changing its native identity. The per-turn `X-Conversation-Request-ID`,
per-message `X-Conversation-Message-ID` and `X-Parent-Conversation-ID` do not
identify the current session. This lets Routing group requests by the same id
the native WorkBuddy session reader already knows; old requests without a
recorded session remain unnamed. See `TestWorkBuddySessionHeader`,
`TestWorkBuddySessionThroughGateway` and `TestRemoteAgent LinkKeepsTheSession`.

DSH names its session in `x-deepseek-harness-session-id`, read the same way
after the others, and marks a request that compacts its context with
`x-deepseek-harness-compact` (#2505). `callKind` records such a call as
`compact`, the Requests list's Compaction; a value of `0`, `false`, `no` or
`off` is no mark, and a request without one is never taken for a compaction
by its prompt. `passOnCaller` sends the mark on to a remote agentlink beside
the session, and no vendor is sent either (`TestDshMarksSessionAndCompaction`).

When the same identity is present in a native session, the native session wins
and the gateway projection is omitted. On `/api/sessions/manage`, in its
counts as in its list, and on `/api/sessions/delete`, a native session is any
session file the agent keeps, one nothing was said in too, with no limit
(`sessions.NativeIDs`); one pass over the agents' files (`sessions.Manage`)
reads only the agent shown and the agents with gateway sessions (#2776).
Gateway projections have no path,
resume command or terminal action: the agent's own files of them are not on
this computer. Their rows carry `gateway: true`; the Sessions page groups them
under "Seen through the gateway" rather than "No folder", and their size is the
text agentlink recorded of them (`sessions.GatewaySizes`), shown only when there
is some. The overview's top summaries (`MergeExternalStats`) carry
`gateway: true` too, and Usage › Sessions names such a row "Gateway session"
where a folder would be, with "(prompt not recorded)" when no text was kept,
so it doesn't read as a native session with an empty prompt (#2285: a Pi
subagent whose session file is in a temp folder). Verification:
`TestTopSessionsMarkWhatOnlyTheGatewaySaw`, `sessions-overview.test.cjs`. Their
models, token totals, start/last timestamps and effective prices come from the
same usage records used by Usage and stats endpoints.

Claude Desktop gateway rows can have a local name even without a CLI
transcript (#1675). [`DesktopTitles`](../../internal/sessions/desktop_titles.go)
reads `claude-code-sessions/*/*/local_*.json` and
`local-agent-mode-sessions/*/*/local_*.json` in Desktop's data folders
(`DesktopDataDirs`, the same `desktopdir.Everywhere` the session readers use,
Claude-3p and Windows' MSIX folders included). The record's `title` names the
CLI ids Desktop's own LocalSessionManager counts as the session's: its live
handles `cliSessionId` and `unarchivedCliSessionId`, its lineage
`preClearCliSessionId` and `priorCliSessionIds`, and, with no `cliSessionId`,
its `sessionId` without `local_`. Prior runs share the current name but remain
separate usage identities. A live handle takes precedence over a lineage one,
and conflicting names at the same rank are unknown. A record that is there but
doesn't read or parse (Desktop midway through writing it) keeps its last
parse; one gone takes its names with it; with none, the row keeps the recorded
first-prompt title or its id. Files are read through `filememo`, and the names
stand for a second (`desktopTitlesHeld`) before the files are looked at again,
so a list that assembles hundreds of native Desktop sessions stats them once
and a rename in Desktop shows within a second. The names reach the native
Desktop sessions (`assemble`), every gateway list (`MergeExternal`), the stats
summaries (`MergeExternalStats`), Routing's `/api/gateway/session-titles`
(`Titles`) and the Context page. It does not grant resume or native transcript
capabilities to a gateway row. Verification: `TestDesktopSessionTitles`,
`TestDesktopSessionTitleIdentity`, `TestDesktopSessionTitleHandles`,
`TestDesktopSessionTitleInStats`, `TestDesktopTitlesReadOncePerList`,
`TestDesktopSessionTitlesOnPages`.

A gateway projection can be deleted (lc on Discord). `sessions.DeleteGateway`
moves its recorded text folders (`gateway-conversations/<date>/<identity hash>`)
into agentlink's session trash under `trash/sessions/gateway/`, with a note
marked `gateway`, and adds `{agent, id, last}` to
`gateway-sessions-hidden.json` in the config directory. `/api/sessions` and
`/api/sessions/manage` leave out a hidden session up to that last request; a
newer request lists it again. The usage ledger is not changed, so Usage,
`/api/sessions/one` and the stats endpoints keep what it spent. A session with
a request in the last minute is refused as active. Restore moves the text back
(items that expired or were cleared meanwhile are skipped; a session recorded
again on the same day is refused rather than overwritten) and unhides it; it
accepts only items that are that session's identity folders in the store.
Delete forever keeps it hidden. The recording's promises hold in the trash:
the hourly cleanup erases trashed gateway text past the seven days, "Delete
saved conversations…" erases it too (the notes stay, so Restore can still
list the session), and `HasGatewayConversations` counts it.
Implementation: [`internal/sessions/gateway_delete.go`](../../internal/sessions/gateway_delete.go),
`Restore` in [`internal/sessions/manage.go`](../../internal/sessions/manage.go), and
[`internal/gui/sessions_manage.go`](../../internal/gui/sessions_manage.go).
All-time statistics include gateway history older than native files. Calendar
activity and output totals use each session's actual request dates, including
intermediate dates; extending the range rebases existing native calendar indices.

Implementation: [`internal/usage/gateway_sessions.go`](../../internal/usage/gateway_sessions.go),
[`internal/sessions/external.go`](../../internal/sessions/external.go), and
[`internal/gui/sessions.go`](../../internal/gui/sessions.go).

## Local Gateway Conversation Recording

The Sessions page offers an explicit, default-off recording switch. Confirmation
discloses that prompts, replies and tool results can contain private files and
code. The switch is a card above the session list that says what is kept and
where (this computer only, up to 7 days and 256 MiB). It shows only where it
applies: on an agent whose list has gateway sessions (`gateway` on a
`/api/sessions/manage` row), in gateway mode, or while recording is on or text
is kept. Its "Delete saved conversations…" shows while
`/api/sessions/manage`'s `recorded` says some text is kept
(`sessions.HasGatewayConversations`, true when the folder can't be read), and
asks in agentlink's own dialog first. `Settings.GatewayConversations` is machine-local consent:
`settings.SetGatewayConversations` updates it under the settings file lock.
Other Settings saves retain the latest stored consent under that lock, so an
older snapshot cannot re-enable recording after it is stopped. `KeepOwn`
also prevents sync/restore from enabling it.
`POST /api/sessions/recording` changes this switch; `clear: true` disables it and
erases only locally recorded gateway content (the store and deleted sessions'
text in the trash), leaving usage and native files.
The existing browser-mode authentication guard protects these APIs; this is an
administrator view, not per-caller-key access control.

[`recordConversation`](../../internal/gateway/conversations.go) wraps
`serveAgent` outside protocol translation and gateway middleware. It records
one client-facing exchange, not each retry, only under the session the client
named, using the same `usage.AgentOf` identity as the usage projection, so
every recorded conversation has a Sessions row that opens it. Codex's direct `/backend-api/codex/responses` relay is captured too.
Internal helper calls, token counting, embeddings, model listings, and compaction
paths that do not pass through these entry points are not recorded.
Recording adds no header and changes none, so routing and prompt-cache
affinity see the request as the client sent it. The existing protocol
readers handle Chat, Anthropic, Responses and Gemini; streaming replies use the
existing decoders, with completed Responses items retaining custom tool calls.
Only content actually sent through the gateway is available. Images are shown
as placeholders; hidden reasoning, client-only operations and unsupported wire
items cannot be reconstructed. This is not a raw request archive or remote
session synchronization, and existing S3 archives are not imported.

[`SaveGatewayTurn`](../../internal/sessions/gateway.go) stores normalized,
scrubbed exchanges in one append-only `turns.jsonl` per
`gateway-conversations/<UTC date>/<identity hash>/` under
the Agent Link config directory, using 0700 directories and 0600 files. Each line is
one bounded turn; legacy per-turn JSON files remain readable until they expire.
An incomplete final append is omitted from reads (marked truncated) and removed
before the next append; complete earlier lines remain available. No headers are
stored. Known secrets and secret-named JSON
fields are scrubbed, but arbitrary sensitive text is not guaranteed removable.
Response capture uses an 8 MiB temporary spool; input is bounded to 8 MiB too.
Normalized turns are bounded to 1 MiB on disk, individual parts to 64 KiB,
and each daily session file to 256 MiB. A full daily session file rejects further
recording that day. Hourly background cleanup removes expired buckets and the
oldest files above 256 MiB; this is a soft store limit between cleanup passes.
Saving a turn does not rescan the whole store, and background traversal and
sorting do not hold the save lock.
Capture, parsing or persistence failure does not change the model response;
filesystem failures are logged without conversation bodies.

Content older than seven days is excluded from reads. The serving gateway
cleans up expired UTC-day buckets at startup and hourly, including with recording
off; the daily layout means physical cleanup can lag the read cutoff by one day.
Disable-and-clear and writes share a lock and recheck the consent generation,
preventing an in-flight request from refilling a cleared store even if recording
is enabled again before that request completes.

`GET /api/sessions/transcript` still prefers a matching native session. Otherwise
it reads a known gateway session's last 128 retained exchanges. A full exact
previous-history prefix is omitted from the next request, while repeated new
messages are preserved. System/tool-definition context is separately collapsed
when unchanged. Compacted, edited or incremental contexts may repeat material;
they are never guessed into a single canonical history. The existing transcript
display bounds apply and truncation is marked. Empty/expired/missing content
has an explicit empty state and all gateway transcripts identify their source.
The page invalidates pending transcript reads on reload or clear. A completed
read updates the current open transcript after redraw, and an older response
cannot replace content fetched after clearing the store.

`GET /api/sessions/markdown` and `POST /api/sessions/export` take the same
choice: a native session is written from its file (`sessions.WriteMarkdown`),
and otherwise a known gateway session from what its transcript reads
(`sessions.WriteGatewayMarkdown`, "Recorded by: agentlink's gateway", with a note
when turns or a part's end weren't kept). A agentlink that others reach as a
remote agentlink has no file of their agents' sessions, so this is the only way
it can export them. Verification: `TestGatewaySessionExportsMarkdown`.

A gateway session without a saved Desktop name is titled by what it was
first asked (YOU on Discord), not its `ses_…` id: `sessions.GatewayTitle` reads the first 8 complete turns of
the oldest recorded file of it in the 7 days kept, takes the first user text
part that isn't a `<…>` context tag, `# AGENTS.md` or an image, and makes it one
line (`untagged`). A finished file's answer is cached by path; a growing
`turns.jsonl` with no title yet is read again. With nothing recorded at all
(the store's folder missing) it costs one stat. `gatewayListed` applies it in
every list (`/api/sessions`, `/manage`, `/one`, the stats summaries); a session
with no saved Desktop name or recorded text keeps its id as title.

`GET /api/sessions/search?agent=&q=` finds an agent's sessions by what was said
in them, for the Sessions page's filter. The query is lowered and its spaces
made one (`sessions.NeedleOf`); under 2 runes it searches nothing. Native
sessions of an agent whose files are read (`HasTranscript`) are read from their
own file (`SearchSession`), first through a byte prefilter (`mayHold`: the
query's longest word, ASCII case folded without a copy, in 1 MiB chunks;
skipped for zstd files, for queries JSON may escape, and for a session whose
path isn't the file its conversation is in, `linesFile`: an OpenCode session's
info file or database, a Cursor chat's SQLite store, whose newest pages sit in
its `-wal`; `search_db_test.go`), then decoded only to
the first match in what the user or the model said. Of a Claude or Codex file
only the lines holding every word of the query are decoded (`readTranscriptOf`,
a part being one line's); Pi's tree is read whole. Without that, one 1.3 GB
Claude session took the whole 8 seconds; with it, all 58 of the owner's (3 GB)
are searched in 3–7 s on a loaded machine. Gateway sessions, and the native sessions of an agent whose files aren't
read (then `unread: true`), are searched in their recorded transcript
(`SearchGateway`). System prompts, tool definitions (`role: system`, `kind:
context`) and images are never matched. Sessions are read newest first by 4
workers at most, stopping at 200 hits or 8 seconds (`cut: true`, with
`searched` of `total`); the request's context cancels the read, so the page's
next query ends the last one. Each hit carries the match with up to 36 runes
before and 120 after, cut ends marked "…". The page waits 300 ms after the
last keystroke, aborts the request in flight, shows a title, folder, id or
model match at once, adds the sessions the search found with their snippet
(the match in a `<mark>`, built as text nodes), and says when it is
searching, cut short or limited to recorded text.
Implementation: [`internal/sessions/search.go`](../../internal/sessions/search.go),
`searchSessions` in [`internal/gui/sessions_manage.go`](../../internal/gui/sessions_manage.go),
[`internal/gui/assets/plugins/sessions/client.js`](../../internal/gui/assets/plugins/sessions/client.js).
Verification: `TestSearchSession*` (`TestSearchSessionReadsOnlyLinesWithTheWords`
for the line filter), `TestContainsFold`, `TestMayHoldSkipsAFileWithoutTheWords`,
`TestGatewayTitleAndSearch`, `TestSessionsSearch*`; browser
`sessions-search.test.cjs`.

Verification: `TestGatewayConversation*` (`TestGatewayConversationNeedsTheClientsSession`
and `TestGatewayConversationClientRequestPrefixedSession` for the two rules above), `TestGatewaySessionHistoryAndCalendar`,
`TestGatewaySessionsNativeWinsBeforeLimits`,
`TestGatewayStatsRebaseNativeCalendar`, `TestCORSKeyThroughTheServer`,
`TestSessionsDeleteGatewaySession`, `TestGatewayDeleteTrashExpires`, `TestGatewayPurgeKeepsItHidden`, `TestGatewayDeleteNothingRecorded` and
`TestGatewayRestoreRefusesForeignItems`; browser
`gateway-conversations.test.cjs`, `sessions-talk.test.cjs`,
`sessions-gateway-delete.test.cjs`, and locale checks.

## Usage over the gateway, and subagents

`GET /v1/agentlink/usage` and `GET /v1/agentlink/usage/requests`
([`internal/gateway/usage_api.go`](../../internal/gateway/usage_api.go)) serve
the Usage page's summary and request ledger from the same builders as the
window's `/api/usage` and `/api/usage/requests`
([`internal/usageapi`](../../internal/usageapi/usageapi.go)), read-only and
without conversation text. They follow `/v1/agentlink/quotas`' guard: this
machine, or another with an enabled gateway key, behind the same Host check.
A key that is held (`heldKey`: a budget, models or accounts) is refused the
summary and gets the ledger filtered to its own `CallerKey`, with its filter
lists cut to its own rows (`usageapi.OwnPage`): the ledger's facets are drawn
from every call in the period, so without the cut they would name other keys.
`TestUsageOverGateway` covers both, and a red when either cut is removed.
A held key is refused (`refuseHeld`, 403 `provider.KeyHeldCode`) the
quotas, their history, kept cards and refresh, and `/v1/agentlink/concurrency`
(#1981); `/v1/agentlink/route` tells it only routes whose `callerKey` is its
own. `Route.callerKey` is set by `trace.beginFor` from the request's
caller, so a route begun without it is told to no held key.
`TestHeldKeyToldNoAccounts` covers every one of these from loopback and
the LAN, for a key held each of the three ways and one held to nothing.
A held key whose owner set `access.Key.Quotas` (#2216) is told the four
quota routes through `scopeQuotas` (`quotas_scope.go`) instead: cards of
providers whose every catalog model `modelAllowed` lets it use, and of
those only accounts `AllowsAccount` lets it use, matched by the card's
`User` against `AccountIDs` (a name two accounts share must be allowed
both; a card no account is known by, only where the list names none of
the provider's). Remote cards (`From`) are never told. A refresh of a
card outside the scope is refused, one naming none reads the scope's
cards only, and a scope with no card answers the cards routes as held.
`TestHeldKeyToldItsOwnQuotas` covers each route for a provider-wide key,
a key with some of a provider's models, an account-held key, a provider's
only key, and a key told nothing, each red when its check is removed.

### Calls read from the agents' own files

A call an agent made without agentlink reaches Usage only when its own files
are read (`sessions.Calls`, from `callSources` in
[`calls.go`](../../internal/sessions/calls.go)). The readers are Claude
Code's, Claude Desktop's and Cowork's, Codex's (and both in WSL), and,
read a file at a time (`wholeCallReaders` in
[`call_cache.go`](../../internal/sessions/call_cache.go)), OpenCode's
(#680), ZCode's, DeepSeek Harness's (`dsh`), WorkBuddy's, CodeBuddy
Code's (#1779; its sessions are on the Sessions page too, from the same
`projects/<cwd>/<id>.jsonl` files read as WorkBuddy's, resumed with
`codebuddy --resume <id>` and deletable, #2657: `TestCodeBuddy`,
`TestCodeBuddySessionsListedUnderTheirName`), MiniMax Code's (#1605), Kimi Code's (#1607,
[`calls_kimi.go`](../../internal/sessions/calls_kimi.go)) and Pi's and
omp's, here and in WSL (#1951,
[`calls_pi.go`](../../internal/sessions/calls_pi.go)). Each skips the
calls the agent made through agentlink's own entry, which the gateway logged
with the account that answered: Kimi Code's are the ones whose
`usage.record` model begins `agentlink/`. Kimi Code's calls are its
`usage.record`s in every agent's `wire.jsonl` under
`$KIMI_CODE_HOME/sessions` (else `~/.kimi-code`), protocols 1.4 and 1.5;
a fork repeats its parent's records, so each is keyed by agent, time,
model and usage and counted once. Pi's and omp's calls are the session
files' assistant messages, and the calls beside them that Sessions counts
too ("usage" and omp's "model_usage" entries, a compaction's, a branch
summary's and a tool result's usage, on the model and provider in use);
the provider `agentlink` (or `dial`) is agentlink's entry, a failed call has no
usage, and a fork's copy of its parent's entries (written before its
header) is the parent's. The other agents the Sessions page reads (Cline,
Qoder, Grok, Droid, Cursor, Hermes, Alma) have no reader yet (#1607). The
Context page reads the gateway's own traces, so it shows none of these
calls. Verification: `TestMiniMaxCodeCalls`, `TestKimiCode*` (the
reporter's real wire files), `TestCodeBuddyCalls`, `TestWorkBuddyCalls*`,
`TestPiCalls*` (Pi 1.1.0's and omp 18.9.1's own files) and
`TestWSLSessionsListed`.

A read of every call decodes each file's cached shard, and over a long
history that is tens of thousands of files (#2278: 68k shards, 415 MB).
Asks for the same calls while one read is under way wait for it and get
copies of its answer (`sharedCalls`), and reads run one at a time.
`AgentCalls` reads one agent's files only. The Sessions overview's Codex
model and effort (`usage.CodexModelEffort`) is built from Codex's calls
and the gateway's records alone, since only those name a Codex actor.
It is built once at a time per period, and an ask while it builds gets
the last answer for the same home. The answer is kept when usage.jsonl is
missing or grows during the build, and up to 8 periods are kept. The page
asks for an overview it is already waiting for only once (`sessOverview`
in `plugins/usage/page.js`). Before this, each ask during the gateway's
writes rebuilt the whole ledger, and the app grew to 85 GB. Verification:
`TestConcurrentCallsShareOneRead`, `TestAgentCallsReadsOneAgent`,
`TestCodexModelEffort*` in `codex_effort_shared_test.go` (the codex-only
answer equals the whole ledger's), and `sessions-overview-once.test.cjs`.

### Calls an app reports itself

`POST /v1/agentlink/usage/requests`
([`internal/gateway/usage_ingest.go`](../../internal/gateway/usage_ingest.go),
#1874) takes calls that never went through agentlink, from an app that sends
them straight to its vendor, so the Usage page covers them too. The body is
one record or an array of up to 5000 (32 MiB at most), in `usage.jsonl`'s
shape: `t`, `agent`, `provider`, `model` and `rid` are required; `in`,
`out`, `cache_read`, `cache_write`, `cache_write_1h`, `reasoning`, `ms`,
`ttft_ms`, `first_text_ms`, `flow_ms`, `status`, `cost`, `effort`,
`session` and `err` are optional.
Any other field, a quoted number, a negative or out-of-range count, a time
more than an hour ahead or before 2000-01-01T00:00:00Z refuses the whole
post with a 400 that names the record (`error.index`) and the field
(`error.param`), and nothing is written. A past `t` is taken as given,
however old, so a client's history can be backfilled.

- **Who may post:** the GET routes' rule, through the same `lanGuard`:
  this machine, or another with an enabled gateway key while shared. On
  top of that a browser page (`fromBrowser`) needs a key wherever it is,
  since `corsGuard` lets a localhost page through keyless (`Origin: null`
  needs a key there too, #2242), and a held key is refused. `TestUsageIngestAccess`.
- **Writing:** `usage.IngestReported`
  ([`internal/usage/reported.go`](../../internal/usage/reported.go))
  appends under the same mutex and to the same file as `usage.Append`,
  each row with `source: "reported"` and no caller key. A `rid` the log
  already has as reported, or one repeated in the post, is skipped. The
  set of reported rids is read from the log itself, so it survives a
  restart: the first post after one scans the log once, keeping only
  lines that carry the reported mark, and each later post reads only what
  was appended since. The reply counts `accepted` and `skipped` and gives
  each record's `index`, `rid` and `status`, so a re-run of a batch is
  safe.
- **Pricing:** a record with `cost` keeps it (`Record.OwnCost`, the
  `rowOwnCost` flag on a packed row). Without one it is priced at agentlink's
  own prices, like a gateway row.
- **Speed:** `first_text_ms` and `flow_ms` (#2637) mean what they do in
  `usage.jsonl` (`Record.FirstText`, `Record.Flow`), so a reported row's
  speed is `usage.DecodeOf`'s, as a gateway row's is, on the server
  (`Row.Decode`, the totals' `DecodeN`) and in `page.js`'s `ledDecode`. A
  row that reasoned tells none without `first_text_ms`; `ThinkPace` and,
  with `flow_ms`, `HeldShare` tell a reply let go in a burst. `first_text_ms`
  may not be before `ttft_ms` (the first text is content too) or after `ms`;
  `flow_ms` is the middle 80% of the content's time scaled by 10/8, as
  `firstToken.flowMs` measures it, so it is bounded by `ms` × 10/8, not
  `ms`. `TestUsageIngestTellsSpeed` (the reporter's record), `TestUsageIngestTimesChecked`.
- **Showing:** reported rows count in every total. They also have a source
  of their own (`SourceReported`): `RequestPage.Reported`, `?via=reported`,
  and a fourth cell and a "Reported" badge on the Requests tab.
- **Nothing live:** a reported row only shows on the Usage page.
  `Record.IsReported` keeps it out of a gateway key's budget and spend
  limit (`budget.counts`), a subscription window's holds (and the log's
  start that decides whether a window can be reckoned, `firstOwn`), the
  Sessions page's gateway sessions, what a session was routed to (`Vias`),
  and Codex's sealed answers and remembered model. Nothing else is done
  for it: no OTel span, no rate limit, no rest, no alert.

`TestUsageIngest*` and `TestIngestReported*` cover these, including
concurrent posts beside the gateway's own writes, and a backfill of a full
5000-record batch landing on the right 7d/30d/all days.
`TestReportedTouchesNothingLive` and `TestUsageIngestTouchesNoLimit` go red
when any of the live guards is removed. Browser:
`usage-ledger-reported.test.cjs`.

With 200k reported rows (a 47 MB log), 40 posts of 5000 took 0.4 s in all.
The first post after a restart took 0.45 s to read the rids back, and a
later one about 2 ms. The Usage page's first read builds the log index
(2.1 s, as it does for the gateway's own rows); after that, a page of all
took 27 ms and the all-time summary 215 ms.

A usage record's `Session` stays the conversation. A Claude Code subagent's
record adds `Subagent` and `ParentAgent`. Through the gateway they come from
the `x-claude-code-agent-id` and `x-claude-code-parent-agent-id` headers
Claude Code sends for a subagent's requests (`subagentOf`, read in
`appendUsage`); the main thread sends neither. A row read from the agent's own
files gets `Subagent` from the file's name, `<session>/subagents/agent-<id>.jsonl`
or `<session>/subagents/workflows/<run>/agent-<id>.jsonl`
(`sessions.SubagentOf`), so no cache version changes, and `ParentAgent` from
the `parentAgentId` of the `agent-<id>.meta.json` beside it, which Claude Code
writes for a subagent a subagent started (`sessions.SubagentParent`, through
`ccMeta`: read once a file; a meta file not read is unknown and read again). A gateway row matched
to a file row keeps the gateway's own. Both were checked against Claude Code
2.1.296 spawning a subagent from a subagent.

A folder these readers go into may be a link: a symlink, or on Windows a
junction, which Go lists as irregular rather than as a folder or a symlink.
Each reader that picks the entries it goes into (a Claude Code, Qoder or
Cowork project folder, a session's own folder and a workflow run's, a Hermes
profile, an Antigravity CLI conversation) asks `dirent.IsFolder`, which
follows such an entry with `os.Stat`; none of them recurses through one. A
walk root that is a link is walked where it leads, its files still named
under the agent's folder (`walkLinked`, #2233). `TestIsFolder` and the
`*BehindALink` tests make real links, junctions on Windows (#2268).

Codex session identity is read from native thread metadata, rather than inferred
from a shared session header or `Kind` (`x-openai-subagent`). The sessions API
adds `role` (`main`, `subagent` or `unknown`), and when native metadata
establishes them, `parent_session` and `root_session`. A child whose root is not
known keeps an empty `root_session`; Codex entries without explicit identity
metadata, gateway-only entries and conflicting identities set `role` to
`unknown` and do not acquire a guessed parent or root. These fields are omitted
for other agents. This is Codex-specific (#1591).

A session whose agent keeps its subagents' conversations beside its own
lists them in `subagents` on the sessions API (`/api/sessions`, `one`, `manage`; #2261,
[`subagents.go`](../../internal/sessions/subagents.go)), only for agents with
a transcript, oldest first. Each has `id`, `title` (the task it was given),
`kind` (the agent it ran as), `parent` (the subagent that started it, empty
for the session) and `start`/`last`; its file is never sent.

- Claude Code, Qoder and Qoder CN: each `subagents/agent-<id>.jsonl` (and
  under `workflows/<run>/`), with `agentType`, `description` and
  `parentAgentId` from the meta file beside it (`ccMeta`). With no meta file
  (Qoder), the title is the file's first user line (`ccTask`, at most 16
  lines read). `agent-acompact-<id>.jsonl` is a compaction's own run and is
  left out.
- OpenCode: a child session (`parent_id`, from the JSON store or
  `opencode.db`) is a subagent of its root, not a session of its own;
  `ocRoots` records its parent. Its title splits 1.x's
  `<description> (@<agent> subagent)`; 2.x titles it the description alone
  (task tool, opencode 2.0.25), and a default `Child session - <time>` is no
  title. OpenCode 2 names the agent in `session_v2.agent` (2.0.29), which
  is the kind when set, the title's `@<agent>` otherwise (`ocV2DB.agent`,
  kept once found; #2290). ZCode shares the parser but has no transcript,
  so lists none.
- Kimi Code: each `agents/agent-<n>/wire.jsonl`. 1.5's `subagent.spawned`
  record, written in the starting agent's wire, gives `subagentName`,
  `description` and `parentAgentId` (`kimiWireOf`, read again only when the
  file's size or time changes); 1.4 writes none, and the title is the
  subagent's first `turn.prompt`. In a subagent's own wire, a `turn.prompt`
  from a `system_trigger` named `subagent` is the task it was given and
  opens as its first user message, not folded context (`kimiTask`, #2290);
  other system triggers, and any in `agents/main`, stay folded.

These are read beside the parsed-session cache, not in it, so its version
is unchanged. `GET /api/sessions/transcript` takes `sub=<id>` for a
subagent's conversation; the id is looked up among the session's own
`subagents` (`sessions.SubagentTranscript`), never used as a path, and one
not listed is an error. See `TestClaudeSubagents`, `TestQoderSubagents`,
`TestOpenCodeSubagents`, `TestOpenCodeV2SubagentKind`, `TestKimiSubagents`,
`TestKimiSubagentTask` and
`TestSessionSubagentTranscript`.

The Sessions page draws only main sessions on top. A session's subagents are
folded under its row ("2 subagents"); unfolded, each is a row of its task and
kind, set in under the subagent that started it, and a click shows its
conversation (`sub=`). A Codex thread whose `parent_session` chain reaches a
listed session is drawn, as a session with its own details, Resume and
Delete, under its parent in the same fold; a loop of parents, a chain deeper
than eight or a parent not listed leaves it on top. The folder's count and
checkbox cover the sessions under others. See `sessions-subagents.test.cjs`.

Codex keeps the existing session-title precedence. Only when that leaves the
title empty may a child thread use its parent's `spawn_agent` `task_name`,
matched by the same `call_id` and an `item_completed` event whose item has type
`SubAgentActivity`, kind `started`, the same `id`, and the child's
`agent_thread_id`. A legacy rollout (Codex's default history before paginated)
keeps that started activity as an `event_msg` `sub_agent_activity` instead, its
`event_id` the call id, and it links the same way. A link counts only from the
file of the child's own parent (`parent_session`): a forked child's file
repeats its parent's history, spawns of its siblings included, and that copy
neither names a child nor takes a name away from one.
The native top-level `session_meta.agent_nickname` is exposed separately as
`nickname` on list, one, manage and overview responses; it does not replace the
title. The parsed-session cache is versioned for nickname and task-title
metadata, and the Codex-call cache for compaction, so older parses are rebuilt.

`GET /api/sessions`, `/api/sessions/one` and `/api/sessions/manage` also return
`model_effort` for each Codex thread, grouped by the canonical Usage ledger's
`model` and effective reasoning effort. It carries that thread's input, output,
cached and cache-write tokens, reasoning tokens, cost, and whether every grouped
call was priced. The Usage ledger's canonical rows supply these values: a gateway
response paired to the native `response_id` contributes once with the gateway
row's `model`, effort, tokens and price, and identical local copies are folded
once.
`/api/sessions/overview` adds the same field to its `tokens`, `cost` and `active`
top summaries using the overview's matching local calendar range; `days=0` uses
all history. The existing `models` field and session statistics keep their prior
aggregation and meaning.
These are fields on the existing API responses; no endpoint or statistics
structure is added.

In the Usage request APIs, `session_role` classifies the canonical Codex actor as
`main`, `subagent` or `unknown`. For a confirmed identity, `session` is its root
thread ID; when the actor is known but its root is not, `session` is empty. An
unpaired gateway row keeps its raw `session` value and has `session_role`
`unknown`. The existing `subagent` and `parent_agent` fields are reused for
Codex: they carry the child thread ID and, when known and itself a subagent, the
parent thread ID. `native_session` stays the recorded native header. The new
`session_role` is omitted for other agents, whose existing field semantics stay
unchanged. Ambiguous or unpaired calls do not inherit a relationship from shared
session metadata.

`GET /api/usage/requests` and `GET /v1/agentlink/usage/requests` expose Codex
`compaction` as `true` or `false` when a native Codex call establishes the
marker state, and omit it when there is no matching native evidence. Only a
call read from a `token_usage_record` (it has a `response_id`) establishes it:
Codex writes that record and a compaction's `compaction_response_id` together,
so a call told from `token_count` alone, in an older rollout, leaves it out
rather than saying `false`. A unique
`response_id` match adds this metadata to the gateway row while preserving that
row's model, usage, cost and kind.
Compaction is already included in Codex's total consumption, so this marker
does not add tokens or cost. This is Codex-only; Claude behavior and the POST
`/v1/agentlink/usage/requests` ingest allowlist are unchanged.

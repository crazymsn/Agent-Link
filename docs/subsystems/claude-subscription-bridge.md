# Claude subscription bridge: turns across Claude Code runs

A Claude subscription's requests are answered by the local `claude` binary
(`internal/gateway/claude_subscription.go`, `serveClaudeSubscription`). This
page covers how a conversation's turns find the Claude Code process, or the
saved session, that had its last turn, and what a run leaves on disk. Tool
calls within a turn (`match`, `continueWith`) are only touched on where they
meet it.

## Which Claude Code runs

`claudeBinary` (and the sign-in's `claudeExecutable`) take
`claudecode.Find`: the machine's own `claude` (`proc.FindTool`), else the
one `Agent Link claude-code install` downloaded into
`<cache>/claude-code/<version>/` (`internal/claudecode`), else, on Windows,
one in WSL. The download is Anthropic's native build from
`downloads.claude.ai/claude-code-releases`, the "latest" channel, as
`claude.ai/install.sh` takes it: the release's `manifest.json` gives the
platform's SHA-256 and size, and a build that doesn't match both is not
kept. It is only made when the user asks (`Agent Link claude-code install`, or
yes to `Agent Link accounts add claude`'s offer on a machine without one), for
a server or a container (Jorben on Discord). It is not on PATH and stands
in for no agent. It doesn't update itself (the bridge sets
`DISABLE_AUTOUPDATER`); installing again takes the newest and removes the
older one.

A run is started with `--dangerously-skip-permissions` (`claudeCLIArgs`):
its only tools are Agent Link's MCP ones, answered by the client, and
Anthropic's web search. Claude Code run as root refuses that flag ("cannot
be used with root/sudo privileges") unless `IS_SANDBOX` is `1` or
`CLAUDE_CODE_BUBBLEWRAP` is set, so a run as root, Agent Link's own euid 0 or
a WSL distro whose default user is root (the probe's `uid`,
`wslrun.Tool.Root`), is given `IS_SANDBOX=1` (`claudeCLI.asRoot`;
FrierenF on Discord). A run as anyone else has its env as before. Only the
bridge's run gets it; the warm-up and `/usage` runs don't skip
permissions.

Every Claude Code Agent Link starts for the subscription (a turn's run, the
warm-up and model test `askClaude`, `/usage`, and the sign-in's
`auth login` and `auth status`) is given its proxy through one helper,
`claudeCLI.proxied` in each package, which is
`netproxy.EnvForBunWith`: the account's own proxy, else the
subscription's, else the global one (`netproxy.EnvWith`), with a SOCKS5
proxy replaced by Agent Link's loopback HTTP bridge to it (`netproxy.Bridge`,
as for every bun Agent Link starts, #1409). Claude Code is built with Bun,
which takes only http:// and https:// proxies: given `socks5://`, every
request failed as `UnsupportedProxyProtocol`, after about three minutes of
Claude Code's own retries, as a 502 (#2651). A Claude Code in a WSL
distro that doesn't share Windows' loopback (`wslrun.Tool.Host` set)
can't reach the bridge, so there, and where the bridge can't start, a
SOCKS proxy is refused before Claude Code starts, with an error that says
to use an http:// proxy. The `settings.json` env that Agent Link writes for the
user's own Claude Code sessions leaves a SOCKS proxy out (see
[Providers and accounts](providers-accounts.md)).

The helper offers each of the client's tools under `bridgeToolName`
(`bridgeTools`), which Claude Code calls `mcp__Agent Link__<name>`. Claude Code
keeps only `[A-Za-z0-9_-]` of an MCP tool's name, so a name it would change
(`spawn.agent`, `fs/read`), one longer than 51 (64 with the prefix) or one
ending as a changed name ends (`_` and 12 hex digits) is cut, made of those
characters and given a hash of the whole name: unique (`a.b` and `a_b`
stay apart), the same in every run, so a resumed session, the history
(`renderParts`) and the tool-choice line (`toolChoiceAsk`) name it as the
run was offered it. `subscriptionRun.names` (`bridgeNames`) maps it back
to the declared name in `toolStart`, for streamed and whole messages, and
in `mcpCall` (#2649).

The run's MCP server is Agent Link itself (`Agent Link claude-mcp-helper`,
`internal/claudebridge`), which hands each tool call back to the gateway
at `POST /_Agent Link/claude-mcp/<token>` (`mcpCall`), where the call is
registered in `b.calls` for the turn with its results to find. The token
is `randomToken` (24 bytes from `crypto/rand`), one per run. The callback
URL is written to `callback` in the run's own temp folder (0600) and the
helper is given that file's path, so the token is never on Claude Code's
command line. The helper calls the address the gateway listens on
(`callbackBaseURL`: `AGENT_LINK_ADDR`, `0.0.0.0`/`::` as `127.0.0.1`), which,
pinned to one interface's address, isn't loopback, and it sends no
gateway key. `lanGuard` therefore lets in exactly that path, by POST, with
the token of a run now live (`ownCallback`: `addCallback` when the run is
registered, `dropCallback` in `removeRun`, compared in constant time).
Every other request, and this path with a wrong, spent or encoded token,
is judged as any other. Without it the call was refused 401, never
reached the run, and every turn with tool results found no run waiting and
started a new Claude Code told the whole conversation (#1546).

## Why turns go on in one run

A run started anew is told the whole conversation in one user message
(`renderClaudePrompt`), a prefix the prompt cache has never seen, so the
conversation is written to the cache again and its images are sent again.
By default, that message holds the history in blocks of `historyChunk`
messages, and Agent Link's one mark (Claude Code keeps three of Anthropic's four) is on the
last whole block, else on the caller's instructions: a later run started
anew for the same conversation within the hour reads the history up to an
earlier run's mark, found within Anthropic's 20-block lookback. A
conversation with no history (no reply that calls no tool, as a sub-agent's
loop of tool calls that Claude Code's AgentSummary asks a summary of) has
its every message but the last chunked instead. The turn to answer after a
history is not chunked, and image blocks count toward the lookback.
A turn that goes on in the run that had the last one is told only the
messages since (`renderClaudeTurn`), and Anthropic reads the rest from the
cache.

## Client-placed cache marks (#1717)

A client can send `X-Agent Link-Cache-Marks: client` on a `/v1/messages`
request to a Claude subscription. Without that header, the bridge behaves
as before. Chat Completions and Responses ignore the header. The mode is
part of the run's identity, including rewritten conversations and tool-result
handoffs: opted-in and ordinary requests never share a run.

An opted-in run sets `DISABLE_PROMPT_CACHING=1`. Its four cache slots belong
to the client. `claude_cache.go` validates the marks before run selection;
`renderClaudePrompt` places them in a new run's first user message.

| Client mark | Bridge behavior |
| --- | --- |
| `cache_control` without `ttl` | Uses `5m`. |
| System block | Ends its own instruction block with the same TTL. |
| Top-level `cache_control` | Sets the request-level tail through `CLAUDE_CODE_EXTRA_BODY`. |
| Last message block | Sets the same request-level tail. |
| Earlier message block, new run | Ends a block at that point in the rendered history with the same TTL. |
| Earlier message block, running or saved run | Dropped and reported; the existing history cannot be edited. |
| Tool definition | Dropped and reported; Claude Code owns the tools. |
| Bridge history-chunk mark | Uses only a free slot and never a longer TTL than a preceding mark. |

`setSafeguards` updates the tail before each turn or tool-result handoff
that changes its TTL, and clears it when a request no longer asks for it.
An explicit last-block mark and a top-level mark both count toward the
request limit. The bridge represents their shared boundary with one tail
when their TTLs match. Different TTLs at that boundary are rejected. Marks retained
in the first user message survive shelving. A new tail that cannot fit
the retained slots or TTL order starts a new session with the rendered
history.

More than four marks, or a `1h` mark after a `5m` mark in tools → system →
messages order, returns HTTP 400 with Anthropic's `type: error` and
`invalid_request_error` shape. The count includes top-level
`cache_control`. Both error messages and the omitted-TTL default were
checked through a relay to the live Anthropic API.

`X-Agent Link-Cache-Marks-Dropped` lists counts by section, such as
`tools:1,messages:3`. It is absent when nothing was dropped. A mark on a
system block removed with the billing header is reported as `system`.
The bridge also reports marks on content it cannot render. Cache usage
retains Anthropic's `cache_creation.ephemeral_5m_input_tokens` and
`ephemeral_1h_input_tokens` values.

The dropped-mark header and its keepalive hold belong only to a Claude
subscription try. An API-key member keeps the client's marks and clears
any report or hold left by an earlier subscription try. Validation still
applies before routing among the members.

The dropped-mark header is best-effort on streaming responses. Validation
and tool-drop counts are known before headers are sent. An opted-in
Claude subscription try waits at most two seconds for run selection to
decide the other drops. Selection releases that hold before process startup or upstream
output. If selection takes longer, the gateway logs the missing report
and sends keepalives without the dropped-mark header. Later fallback can
also change the selected run after headers have been sent. Reporting never
fails the request or delays an ordinary request's keepalives.

## The caller's instructions as the system prompt (`claudeLeanPrompt`)

Every run is the real Claude Code. By default it keeps Claude Code's own
system prompt, and the caller's instructions go in the run's first user
message inside `<external_system_instructions>`. Settings › Models ›
Replies › Short Claude system prompt (`claudeLeanPrompt`, off by default)
starts a run with `--system-prompt-file` instead. The file is `system.md`
in the run's temp folder, under the distro's mount in WSL (`claudeCLI.file`).
It holds `leanSystem`: the caller's instructions without a client Claude
Code's billing line, plus the tool-choice line. The first message then holds
the conversation alone. Claude Code still sends its billing block and its
identity line, so requests come from the real binary. They no longer carry
Claude Code's prompt, and that is the ban risk the setting names.

A request with client cache marks (#1717) is never lean: its system marks
are placed in the first message. The mode is part of the run's identity
(`cacheIdentity`, and the tool-results check in `serveSubscription`), so a
run started in one mode never takes a turn of the other. After the setting
changes, the next turn starts a run with the rendered history.

Measured on 2026-10-16 with a real Claude subscription on Sonnet 5.5,
Claude Code 2.1.301:

| Client | Off | On |
| --- | --- | --- |
| Chat client, ~600-token prompt, 2 tools, 3 turns | 10,048 input tokens | 6,580 (−35%) |
| `claude -p` as the client, Read and Bash, 4 requests | 85,453 / 85,471 | 81,948 / 81,977 (−4%) |

The saving is a fixed ~0.9–1.2K tokens a request. It is large for light
clients and small for clients with Claude Code-sized prompts. Without any
tool, Claude Code's built-in tools are already off (`--tools ""`). A run
told the whole conversation again costs far more, because it writes the
history to the 1h cache at 2×.

## Runtime path for a new turn

`serveClaudeSubscription`'s `start` tries, in order:

1. `subscriptionBridge.resume`: an idle run waiting at the conversation the
   request goes on from (`b.idle`). It is written the turn's messages on its
   stdin, as a user message with a uuid it is rewound to if the client gives
   up on the turn (`letGo`).
2. `subscriptionBridge.unshelve`: the saved session of a run let go past
   `idleMost` (`b.shelf`, at most `shelfMost`), which a new Claude Code
   starts from with `--resume`, told the turn's messages alone. It is
   sent `initialize` first, which Claude Code answers only once it has
   loaded the session; one that can't load it exits 1 ("No conversation
   found with session ID") without answering. `start` then lets the
   session go and goes on to step 3 in the same request, logged "Claude
   Code could not go on from the conversation's saved session". Before,
   the turn got 502 "Claude Code ended" with the safety context set, and
   an empty reply without it.
3. `retire`, then a new run told the whole conversation. When it has
   replies in it, the turns already answered are wrapped in
   `<conversation_history>`, with a note that the images and files in them
   were sent with those messages, and the turn to answer (from the message
   after the last reply that calls no tool, `historyEnd`) in
   `<current_turn>` (#1365). Told as one stretch of Human:/Assistant: text,
   earlier images read as just sent. A first turn, or messages with no
   reply among them, are told as before. `start` logs "a new Claude Code
   is told the whole conversation" with the message and image counts.
   Claude Code notes every image of the message after it, so after the
   turn to answer: `[Image: source: …/images/N.png]` where it keeps the
   images (the session kept on disk), or the size it showed a large one
   at. When the history has images, the turn's opening says those notes
   are the history's, or the history's first and then the turn's
   (`currentTurnOpen`, #1751). Told nothing, the model said a text-only
   turn had an image attached and offered to open the path.

A Claude Code that ends before it answers a control request
(`cliControl`: `initialize`, the safety context) is reported with its
exit status and the end of its stderr (`endedBefore`), as one that ends
before it reads its input is (`whyEnded`). The tool-results path and
`resume` already start a new run when the one they found has ended.

Agent Link ends a run (`abort`) by asking Claude Code to end with SIGTERM
(`proc.Tree.Stop`) and kills its tree only once it has exited or
`stopGrace` (5s) has passed; Windows kills the job at once. Claude Code
removes `~/.claude/.oauth_refresh.lock` as it exits on SIGTERM (2.1.299
exits within half a second), and the SIGKILL Agent Link sent before left the
lock for the next requests to fail on (#2195). The run counts as ended at
once; the gateway stopping (`abortAll`) waits for every tree. Covered by
`TestClaudeAbortAllowsLockCleanup` and `TestTreeStop*`.

## A side question after a reply (#1638)

After each turn Claude Code 2.1.290 asks its prompt suggestion: the
conversation so far, with the turn's system prompt, tools and thinking so
it shares the cache, and one more user message, "[SUGGESTION MODE: …",
with no system message after it. Its request goes on from the turn's
reply like a next turn, so `resume` gives it the conversation's run, and
the run waited at the suggestion's reply. The user's real next turn then
found no run and was told the whole conversation in a new Claude Code
(the reporter's 41 messages, 88k tokens written to the cache, minutes
after the last reply, with nothing in the log).

`ended` now keeps, beside the run's new key, the key it went on from
(`behindKey`) and the uuid of the turn that took it there (`behindTurn`).
A turn that finds no run under its own key takes one whose `behindKey` is
its key, rewinds it to `behindTurn` (as `letGo` does) and tells it only
the turn's messages ("taken back from the side question it answered
since"). The run is marked `rewound`, so it isn't shelved. A run that
can't be rewound is let go and the turn told whole to a new run, as
before. Tried with the real 2.1.290 client in `-p --prompt-suggestions`
and the real 2.1.290 as the run, against a local mock of Anthropic: one
run for the whole session, and the turn after each suggestion went on
from the reply before it. Other side requests that go on from the
conversation (extract_memories, away_summary, /btw) take the same path;
they weren't tried.

A turn after a reply that starts a new run logs why none waited
(`whyNoRun`, "claude: no run waits for this turn: …"): its run went on
with a turn this one doesn't follow, waits under another key (the model,
settings, system prompt or tools differ), waits at an earlier turn, is
busy with another request, ended (how long ago, and why: `endWhy`, set by
`abortFor`, kept in `b.lost` by conversation), or no run here has had the
conversation. Tool results that find no run waiting add what became of
the run that made the calls ("the run that made the calls ended …",
`whyCallsLost`, from `b.lostCalls`, which `removeRun` fills with the calls
the run waited on and the results it was last handed).

## A turn the client gives up on (`letGo`, #780, #1365)

When the client goes away mid-reply, a run resumed for the turn is told to
rewind (`rewind_conversation`) to the turn's user message, and waits at the
conversation before the turn (`backKey`). The turn's id lasts until its
reply ends without calling a tool: `ended` keeps `turnUUID` and `backKey`
over a reply that calls tools, so a client that goes away while the run
answers its tool results also has the whole turn taken back, its tool
rounds with it. Its next request (the same results sent again, or the next
turn) then finds no run waiting on the calls ("tool results no run
waiting"), is resumed in that run at `backKey` and told the turn since its
user message (logged "a turn taken back goes on in its run"). Before, the
turn's id was dropped with the reply that called the tool, the run was
ended, and that request was told the whole conversation in a new run.
Claude Code 2.1.295 rewinds past a tool round mid-reply (tried against a
local mock of Anthropic). A run started anew for the turn has nothing to
rewind to and is still ended; so is one with a call still in the client's
hands (`pending`). Rewinding it to its first message would leave nothing,
and its results were already handed over its MCP calls, so the results
sent again go to a new run told the whole conversation (#1638's first
case: a 499, then "tool results no run waiting" and 17 messages told).
Keeping its cache would mean taking it back to Claude Code's own
tool_result message and telling the results again as text, which wasn't
done: the reporter's requests aren't known, and a fork that took the
lead's calls (`claimResume`) looks the same from here. The log now says
which run ended and why.

### One key per waiting run

A run waits under one key (`keepIdle`, the only writer of `b.idle`);
kept again, a run leaves the key it waited at, unless another run now
waits there. Which order of calls kept one run twice in the reporter's
gateway isn't known: `ended` and `letGo` of one request exclude each
other (`serveSubscription`'s ended callback runs only when `gone()`
stopped the `AfterFunc` that calls `letGo`), so it isn't those two of one
request. `keepIdle` holds the rule whichever caller it was (#1596). Past `idleMost`, the
eviction loop takes out the key it found as oldest, not the run's own.
Before (X, Sma1lboy: Agent Link at 158 GB after two days of omp agents), one
run under two keys was the oldest on every pass, the pass took out only
its newer key, and the loop never ended: it held `b.mu`, stopping every
Claude subscription request, and grew its list of runs to let go without
bound. `claude_idle_test.go` (`TestClaudeIdleEvictionEnds` runs the loop
in a child process, which on the old code fails on what the loop
allocates, not on a deadline).

`keepIdle` refuses a run that has ended, and its callers then leave the
run's timer alone (#1667). A resumed turn's 30-minute timer can end a run
(`abort` → `finish` → `removeRun`) while its reply is going out, and the
reply's last event, already in the segment, still reaches the client
whole, so `ended` is told it went well. Before, the ended run was kept,
held one of `idleMost` places until pushed out, and its re-armed timer
found it closed and never took it out. `finish` sets `closed` before
`removeRun` takes `b.mu`, and `keepIdle` reads it under `b.mu`, so a run
is either refused or kept and then taken out by `removeRun`.
`TestClaudeRunEndedMidReplyIsNotKeptIdle` fires the timer before `ended`.

## Finding the run: keys

- `nextTurn` splits a request at its last reply that calls no tool: the
  conversation up to it, and the messages since (with any calls and results
  the client made itself after it).
- `ended` keeps a run whose reply was whole under
  `turnKey(owner, req, messages + reply)`: the account, model, whether an
  effort was asked, thinking off, tool choice, system prompt, the tools, web
  search, schema, and the messages as `hashMessages` reads them. The
  system prompt is taken without the client Claude Code's
  `x-anthropic-billing-header` line (`withoutBillingHeader`), which the run
  is never told and whose fields (`cch`, `cc_prompt_id`, `cc_prev_req`…)
  change every turn; with it in, no next turn found its run (#1638). The
  exact match is that key, from `nextTurn`'s split of the next request.
- It also keeps a `looseTurn`: the same settings without the tools, the
  message count, the conversation before the last turn (hashed; its system
  prompt also without the billing line), and the
  last turn's messages one by one (`messageWords`), the reply last. Shelving
  copies it, with the tool names the run's agent had, to the
  `savedSession`.

### A conversation the client rewrote (#912)

Some clients send a turn's conversation back changed. Alma takes the
`[Context: …]` block it gave the latest user message out once the turn is
over, and its `<alma_notification>` block out of the end of the reply, and
picks the tools for each turn. When the exact key misses, `rewritten`
compares the request's `looseTurn` with every idle run's and saved
session's. The request matches one when all of these hold:

- the same owner, settings and conversation before the last turn
  (`looseTurn.key`);
- the last turn has the same messages, roles, tool calls and results, and
  only these changed (`looseTurn.rewrote`):
  - a user message had one part taken out (`cutFrom`), not all of it;
  - the reply is a non-empty start of what the run said.

It is taken only when exactly one run or saved session matches. Two that
could be it are two conversations alike as far as their words go, and a
run handed the other one would answer from what that one was told.

An idle run is taken only when its agent was told every tool offered now
(`offers`). Claude Code is not told of new tools at a turn's start, only
with tool results. A saved session is taken only when the tools offered now
include every one its agent had (`toolsCover`). It is resumed with the new
list, and its earlier calls name no tool that is gone.

The run goes on with the conversation as it told it. Its Claude Code still
holds the context block and the notification. The client's next turn
brings its own context.

### Images the client took out (#1382)

Hana replaces each image before the last reply with a line of text
(`[图片已省略：…]`) in the tool result or user message that held it. Over
Chat Completions, pi-ai sent a tool's images in a user message after the
results ("Attached image(s) from tool result:" and the images); that
message is gone, and each result's text has the line added to its end. So
the step after one that read an image missed the run both ways: its tool
results failed `follows`, and its next turn failed `turnKey` and
`looseTurn.rewrote` (the message count changed).

`lostMedia` (`claude_lost_media.go`) tells this rewrite by its shape, not
by Hana's words. It compares the conversation the run had (kept as each
part's hash, `heardRuns`) with the request's, a role's messages in a row
at a time, and takes it when images or files were taken out and nothing
else changed:

- each image or file is still there, gone, or has text in its place;
- a tool result has text added to its end only when it carried images, or
  the images after the results went;
- the text before those images goes with them only when a result before
  it had text added;
- at least one image or file was taken out.

`follows` checks it when the hash misses (`toldRuns`, logged "tool results
go on in their run, the images before them taken out by the client"), and
`looseTurn.rewrote` when `cut` doesn't match; the looseTurn key no longer
has the message count, which `cut` compares itself. The run's Claude Code
still holds the images; it is told only what came since.

A client that drops the images' message without adding text to a result
goes on only when that message held images alone, no text. A tool result of an image alone (pi-ai's "(see attached image)")
whose text is replaced by the line is not this shape, and starts anew.

## Files a run leaves

- Its session: `<config>/projects/<work project>/<session>.jsonl`, for
  `--resume` (`claudeSessions`). It is removed when the run ends unless
  shelved, and when a saved session is discarded (`removeSession`).
  `sweepSessions` removes those a stopped gateway left, once older than
  `idleLongest`.
- Claude Code's temp files for the session, the images it was given:
  `<CLAUDE_CODE_TMPDIR or /tmp>/claude-<uid>/<work project>/<session>`
  (`claudeTempDir`, not on Windows). They are removed with the session
  (`removeSession`). The sweep removes those whose session file is gone and
  in which nothing changed for `tempLongest` (a day). The folder is
  the work folder's for every account, and another account's session files
  are in its own config folder.

A run started anew still sends every image in the conversation. Replacing
earlier images with a placeholder would change what the model sees, and
Agent Link doesn't do it.

## Tool results another request answers first

A request with tool results finds the run waiting on those calls (`match`)
and claims it (`claimResume`) until the reply it is resumed for ends. More
than one request can carry results for the same calls: Claude Code's fork
sub-agents each start from their lead's conversation as it stands, its
reply's calls answered with the placeholder "Fork started — processing in
background" and the fork's directive after, and they come at once with the
lead's own next turn; a client may also send a turn again while the first
is still answered. The first to claim the run goes on in it. Each of the
others, and one whose run has ended, is a conversation of its own from
there and gets a run started anew (`answeredElsewhere` in the log). It is
never refused: Claude Code doesn't retry a 409, and the agent that sent it
stops (ylorn on Discord).

## The user's words beside tool results (#872, #1878)

A user message can hold tool results and, after them, text blocks of the
user's: Claude Code's AgentSummary asks for its sub-agent's progress label
that way ("Describe your most recent action in 3-5 words. Do not use
tools.", JinJieBeWater), a fork its directive, and an older Claude Code a
message sent while a tool ran (#872). The words are the user's, and both
ways a run is told them keep them apart from the tool's output:

- **A run told the messages as text** (`renderParts`, for a run started
  anew and for a resumed turn): a text or file part after a tool result in
  the same message closes the result, `[end of tool result id=…]`, and is
  marked `[user message]`. Before, it ran on after the tool's last line
  (`SOURCE_LAST_LINEDescribe your…`), and the model read it as tool output
  and went on with the task. A result followed by nothing, or by another
  result, is told as before.
- **Results that go on in their run** (`match`, `continueWith`): `match`
  no longer adds the words to the last result's text, which made them part
  of the MCP result. `saidWithResults` takes them, and `continueWith`
  writes them to Claude Code's input as a user message (`queue`) before it
  hands the results over, and waits for Claude Code's `command_lifecycle`
  "queued" for that message's uuid (at most `queuedWait`, 2s; a version
  that doesn't say goes on after it). Claude Code then tells its model the
  message with the result, as one the user sent while it worked ("The user
  sent a new message while you were working: …"), within the turn. Tried
  with the real Claude Code 2.1.296 against a local mock of Anthropic: the
  next request holds the tool's output, then that reminder, and the turn
  ends with one `result`. Handed over after the results, a message could
  come after the model's reply and start a turn of its own.

What the client's agent puts beside the results is not the user's (#2060).
Claude Code 2.1.296 sends its hooks' notices ("PreToolUse:Bash hook
additional context: …"), its `<total_tokens>` count and a message the user
sent mid-turn as one `{role: "system"}` message after the results; older
versions send them as `<system-reminder>` text blocks in the results'
message. `saidWithResults` takes from a system message only the words
after "The user sent a new message while you were working:" (or "The user
sent the following message:"), without Claude Code's ask around them, and
leaves out a text that `agentNotice` knows: empty within its reminder
tags, a hook's notice of a known kind, a token count, or a lead `userWords`
drops (`injected`). Before, nearly every call of a client with a hook was
followed by "The user sent a new message while you were working:" and the
hook's output, or nothing (the run's Claude Code doesn't show a bare
`<total_tokens>`). `renderParts` closes a result before such a notice
without marking it `[user message]`. A reminder with other words in it is
still the user's, as #872's was.

Where the result ends is said by the message's parts alone. A tool whose
output says "Do not use tools." has it told as its output, inside the
result. The words of a message that holds images (pi-ai's "Attached
image(s) from tool result:" over Chat, #1382) still go with the result
before them, with the images.

A request with words beside the results still takes the run waiting on
the calls when it comes first (`claimResume`). An AgentSummary that does
is answered in the sub-agent's run, and the sub-agent's own next request
then gets a run started anew, as any request after another answered the
calls does. Starting such a request anew instead would tell the whole
conversation to a new run, a prefix the cache has never seen (#1716,
#1717), for every message sent mid-turn.

## The account's allowance on the reply (#1257)

Claude Code tells a run's account allowance in its stream-json
`rate_limit_event`, not as HTTP headers. The bridge keeps it
(`provider.NoteClaudeLimits`) and says it again on the reply as the
`anthropic-ratelimit-unified-{5h,7d,7d_oi}-{utilization,reset}` headers
Anthropic sends (`limitHeaders`, from `provider.KeptClaudeLimits`): a share
from 0 to 1 and a Unix-seconds reset, for each kept window that says when
it renews. They are set when the request comes in, from what is kept, and
again as the reply begins (`relay`'s `begin`), once the turn's own
`rate_limit_event` is in. A run on Claude Code's own home that has moved to
another account says nothing (`ClaudeCodeMovedOff`), as its event isn't
kept either.

A Claude Code signed in to claude.ai (its sign-in field set to
`claudeai`, `claudeSignIns` in `internal/agent/claude.go`) reads these into its status line's `rate_limits`, which
claude-hud shows. One that signs in to Agent Link with Agent Link's key
(`ANTHROPIC_AUTH_TOKEN`) ignores them: Claude Code 2.1.293 reads the
headers only for a claude.ai subscriber. A reply another provider answered
carries none.

## Verification

```sh
go test -tags nogui ./internal/gateway/ -run '^TestClaudeClientCache' -count=1
go test -tags nogui ./internal/gateway/ -run '^TestClaudeLeanPrompt' -count=1
go test -tags nogui ./internal/claudecode/ -count=1
go test -tags nogui ./internal/gateway/ -run '^TestClaudeCodeGivenTheBridgeForASOCKSProxy$' -count=1
go test -tags nogui ./internal/provider/ -run '^TestClaudeSignInBridgesASOCKSProxy$' -count=1
go test -tags nogui ./internal/netproxy/ -run '^TestEnvForBunWith' -count=1
go test -tags nogui ./internal/gateway/ -run '^TestClaudeToolNamesComeBackAsDeclared$' -count=1
go test -tags nogui ./internal/gateway/ -run 'ClaudeSubscriptionRunsTheDownloaded|ClaudeSubscriptionWithoutClaudeCode|ClaudeRewritten|LostMedia|ImagesTakenOut|RunLetGoWhenTheClientRewrote|ClaudeSessionTempFiles|ClaudeLetGo|ClaudeSessionFiles|ClaudeOldSessions|ClaudeSubscriptionReplySaysTheAllowance|ClaudeAllowanceHeaders|ClaudeForksAnsweringTheLeadsCalls|ClaudeTurnGivenUpOn|ClaudeToolResultsGivenUpOn|ClaudePromptMarksEarlierTurns|ClaudeHistoryCachedAcrossNewRuns|ClaudeUnloadableSession|ClaudeEndedRunSaysWhy|ClaudeIdle|ClaudeSideQuestion|ClaudeNoRunSaysWhy|ClaudeToolResultsNoRunSaysWhy|ClaudePromptToldUserText|ClaudePromptKeepsInstructions|ClaudeToolResultsGoOnWithTheUsersWords|ClaudeToolResultsKeepInstructions|ToolResultKeepsUserTextSentMidTurn' -count=1
```

`claude_rewritten_test.go` has a case for each relaxation and one for each
rule that keeps another conversation out; `claude_lost_media_test.go` does
the same for images taken out, from Hana's request bodies. `claude_resume_test.go`'s harness
stands in for Claude Code with a script that keeps sessions as it does.

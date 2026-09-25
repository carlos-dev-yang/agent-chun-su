# WEB exploration proposal — public web research from EC2

**Status:** WEB-01 public read-only host actions implemented on 2026-09-25,
disabled by default on existing installations. The user authorized code and
Git publication. Local page open and private-address denial were observed.
Live Brave search, paired Telegram/EC2 acceptance, browser pilot and route
comparison remain open; no browser package or provider credential was installed.

## Current boundary and deployment facts

- The live EC2 installation was recorded on 2026-09-19 as Amazon Linux 2023
  **amd64**, not Ubuntu. This is recorded evidence, not a fresh host probe.
- The reception Codex process deliberately sets `web_search="disabled"`,
  `permissions.chunsu_mail.network.enabled=false`, `mcp_servers={}` and disables
  browser/search features. It rejects native tool events. Installing a browser
  alone cannot make chat search the web.
- Reception already uses a host-validated JSON action loop with at most four
  actions per turn. Any web capability must be an explicit host action with
  bounded input and output. The model must not gain shell, arbitrary MCP or
  unrestricted network access as a side effect.
- The installed self-updater does not install OS packages or change Codex,
  credentials or system policy. Browser dependencies need a separate operator
  preparation and host validation step.

## Research conclusion

The user-mentioned **Aside** appears to be the Aside AI Browser. Its official
developer guide documents CLI, MCP and REPL, but currently lists installers for
macOS and Windows; it does **not document** a Linux/AL2023 install. This is an
absence of documented support, not proof that Linux can never be supported.
Aside's own article reports up to 80% fewer tokens than agent-browser and a 70%
smaller snapshot on one page. Those are vendor measurements of a browser output,
not a comparison of total tokens, cost or successful tasks in Chun-su. Its
published web-agent benchmark also combines the browser, model and agent
harness, so it cannot decide the backend here.

For this EC2 node, use a **search → public page text → interactive browser**
ladder. A search API returns candidate URLs; a constrained HTTP reader checks
the source text; a headless browser is reserved for JavaScript-rendered or
interactive pages. This is a proposal to measure, not an assumed token saving.

| Candidate | Use in this plan | Constraint |
| --- | --- | --- |
| Search API + Go HTTP reader | First route for public research and citations | No JS interaction; provider key, terms and cost need selection |
| agent-browser + Chrome | First browser pilot for compact snapshots and element refs | Extra binary/browser runtime; AL2023 process and sandbox behavior need a live probe |
| chromedp + Chrome | Go-native comparison or replacement | Requires Chun-su to build and maintain compact snapshot/ref behavior |
| Aside | Possible later desktop/browser connection | Linux installation not documented; token claim does not establish total cost |
| Browser Use / Lightpanda | Optional later comparison | Cloud/Python coupling or browser compatibility and license questions |

Vercel's agent-browser ships native Linux x64/ARM64 binaries and supports
interactive-only, scoped and delta accessibility snapshots plus `read` for
text-heavy pages. It therefore offers a quick way to test the desired agent
interaction with little implementation. chromedp is a Go CDP client and fits
the existing stack, but Chun-su would have to implement the model-facing
snapshot/ref contract. Keep the host action contract backend-neutral and choose
the browser engine only after the AL2023 pilot. The actual EC2 is amd64, so
the arm64 case is a future portability check, not a claim about this node.

## Proposed work units

Owner setup is in [WEB_RESEARCH.md](../setup/WEB_RESEARCH.md), and checks
actually run are in [WEB_RESEARCH_2026-09-25.md](../validation/WEB_RESEARCH_2026-09-25.md).
The provider is Brave Search API;
the default local spend guard is 20 attempted searches per UTC day. The owner
must provision the key locally and explicitly enable web. The key and full
retrieved text do not enter action traces. Since actual AL2023 sandbox and egress
are unverified, WEB-02 is not activated by the executable update.

### WEB-00 — scope, provider and baseline

1. Confirm the first release is **public, read-only** web research. Keep
   authenticated sites, form submission, downloads and account actions out of
   this release. Choose the initial search provider and spending limit; Brave
   Search API is a concrete candidate with a documented web-search endpoint.
2. Prepare a small, human-reviewed task set covering static pages, recent
   information, source disagreement, JavaScript pages, multi-step navigation,
   and denied/private destinations. Record current failure behavior and the
   selected model's token usage on these tasks.
3. On the actual AL2023 amd64 host, inspect available memory, architecture,
   browser dependencies, process sandbox and outbound policy. Run a disposable
   non-root Chrome/agent-browser probe without changing Chun-su services or
   connecting accounts. Record exact versions, resource use and cleanup.

**Exit evidence:** selected public scope/provider/budget, task set and native
host compatibility report. Browser activation waits if sandbox or metadata
egress cannot be constrained on this host.

### WEB-01 — bounded search and source reading

1. Add typed `web_search` and `web_open` actions to the conversation schema,
   channel capability list, host dispatcher and audit. Validate query/URL
   length and scheme, response size, redirects and deadlines before dispatch.
   Keep Codex's native web and shell tools disabled.
2. Put the search provider behind a replaceable Go interface. Keep its key in
   protected local configuration, outside prompts, Git and action traces.
   Return a small set of titles, URLs, dates and snippets, then fetch selected
   source pages over HTTPS. Prefer official/primary pages where available.
3. Prevent requests to loopback, RFC1918, link-local and EC2 metadata addresses,
   including redirects and resolved IP changes. Strip page instructions from
   authority: source text is evidence, never a host command. Return bounded
   text with canonical URL, retrieval time and truncation status. Preserve
   source URL/hash and action outcome in private evidence without logging
   secrets or uncontrolled full pages.
4. Adjust the reception action budget only as supported by the task set. Give
   incomplete or unavailable pages an explicit result and let the user see
   when a claim could not be verified.

**Exit evidence:** real public search/open/reply with clickable citations in
both local chat and paired Telegram; denied local/metadata targets and redirects;
bounded output; existing affected Go checks and manual request cancellation.

### WEB-02 — browser interaction pilot

1. Install a pinned, verified Chrome and agent-browser candidate as a separate
   operator step. Wrap fixed operations (`open`, `snapshot`, link navigation,
   `read`, `close`) with structured arguments. Test `fill` only in the disposable
   pilot; it is not a public-release capability. Never pass model text through a
   shell. Give each request its own browser session and clean up on success,
   cancellation and crash. Keep browser profiles ephemeral in the first release.
2. Run the browser as a non-root isolated worker with a restricted filesystem,
   bounded CPU/memory/time and outbound policy that blocks internal networks
   and EC2 metadata even from page scripts or subresources. Check the actual
   AL2023 sandbox behavior before enabling public browsing.
3. Send compact accessibility snapshots to the model: interactive elements
   first, scoped content or delta next, screenshot only when text does not
   describe the page. Re-observe after navigation before acting on element refs.
   Reject ambiguous controls and any action that could post, purchase, change
   an account or otherwise write. Expose neither arbitrary JavaScript
   evaluation nor profile/auth operations in this release.
4. Compare a minimal chromedp adapter on the same tasks if the extra binary,
   process lifecycle, snapshot fidelity or security boundary proves costly.

**Exit evidence:** dynamic-page and multi-click public reading, source URLs,
clean browser process shutdown, private-network denial and recovery after a
failed navigation. Keep this pilot disabled by default until its host checks pass.

### WEB-03 — efficiency and rollout decision

Measure each route on the same tasks, model and output requirements: task
completion with correct source support, total model input/output tokens,
**tokens and money per successful task**, browser/search calls, elapsed time,
peak memory and failure reasons. Include retries and screenshots in the total.
Compare search+HTTP, search+agent-browser and search+chromedp rather than
equating snapshot length with total cost. Select the default from these results.

Update the setup/runbook and package checks for the selected dependencies.
Deploy with an explicit owner setting, verify a real EC2 Telegram request and
service recovery, and retain a way to disable the web capability without
affecting chat, controller or worker. Do not treat this proposal as permission
to install packages, register a paid API, alter egress or deploy.

## Decisions and remaining gates

- First release is public read-only; authenticated browsing remains a separate
  later decision.
- Brave Search API is selected. The owner supplies its key through the host
  credential store; 20 attempted searches per UTC day is the default local cap.
  A provider-side spending limit and real keyed acceptance remain outstanding.
- Action evidence retains input digests and URL/hash metadata, not retrieved
  page bodies. The reception model sees bounded text in its live turn.
- Actual AL2023 browser isolation/egress mechanism and agent-browser versus
  chromedp choice wait for a disposable host probe. Neither is enabled now.

## Primary sources checked on 2026-09-25

- [Aside developer CLI/MCP/REPL guide](https://docs.aside.com/help/developers)
  and [Aside token-efficiency article](https://aside.com/blog/how-we-built-the-sota-browser-agent-that-outperforms-fable)
- [Aside benchmark artifacts](https://github.com/at-inc/aside-benchmarks)
- [agent-browser README](https://github.com/vercel-labs/agent-browser),
  [agent-facing snapshot guidance](https://github.com/vercel-labs/agent-browser/blob/main/skill-data/core/SKILL.md)
  and [Linux release build checks](https://github.com/vercel-labs/agent-browser/blob/main/.github/workflows/release.yml)
- [chromedp README](https://github.com/chromedp/chromedp)
- [Playwright container/browser sandbox guidance](https://playwright.dev/docs/docker)
- [Brave Search API](https://brave.com/search/api/)
- [Browser Use](https://github.com/browser-use/browser-use) and
  [Lightpanda](https://github.com/lightpanda-io/browser)

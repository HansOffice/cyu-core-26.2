# CyuCore Plugin API Foundation Log

This log records the first public plugin-platform milestone. It supplements `docs/DEVLOG.md`, ADR 0001, ADR 0002 and Git history so a future maintainer can recover both the implementation sequence and the constraints behind it.

## Scope

The goal of this branch is **not** to choose a dynamic loader. The goal is to define one loader-independent semantic API and host runtime that future built-in Go, WASM, or other adapters must obey.

The public boundary intentionally excludes `Server`, `PlayerSession`, `net.Conn`, packet IDs, protocol-v776 structures, internal maps and mutable world/player implementation objects.

## Implementation sequence

### `17721ea` — lifecycle API and host manager

- Added Plugin API v1 identity, descriptor validation, logger and lifecycle contracts.
- Added deterministic registration/enable/reverse-disable order.
- Added failed-enable cleanup and rollback.
- Converted plugin panics into host errors instead of process crashes.
- Added lifecycle timing/state diagnostics.

### `92977b7` — owner-scoped EventBus

- Added public event subscription contracts while keeping dispatch host-only.
- Event subscriptions belong to one plugin owner and are automatically removed during disable/rollback.
- Event dispatch uses a copy-on-write immutable handler table so normal dispatch does not copy the subscriber list or take the mutation mutex.
- Handler errors/panics are isolated per plugin and do not abort later handlers.
- Added event call/error/duration diagnostics.

### `ecf9bd1` — public player event contracts

- Added immutable public Player snapshots instead of exposing `PlayerSession`.
- Added PlayerJoin, PlayerQuit and cancellable/mutable PlayerChat events.
- Block interaction events were deliberately deferred because the core still lacks a stable public `ResourceKey`/`BlockState` model; numeric protocol/runtime block IDs are not acceptable public API.

### `83bb7e3` — server lifecycle integration

- Plugin enable now happens after listener bind succeeds and before server runtime/network work begins.
- Join/Quit/Chat dispatch runs through the server runtime owner.
- Shutdown drains registered-player lifecycle work before stopping tick execution and disabling plugins.
- Added real Server-to-plugin integration tests including chat cancellation.

### `2b88ca9` — tick-owned Scheduler foundation

- Added `AfterTicks` and `EveryTicks` public scheduling contracts.
- Host scheduler uses a min-heap ordered by due tick + stable task ID rather than scanning every task each tick.
- Added owner cleanup, cancellation, handler panic/error isolation, deterministic ordering and per-tick execution budget.
- Scheduler task callbacks are stored only inside pluginruntime; they are **not** runtime mailbox work items and do not weaken ADR 0001.

### `af32424` / `77983c6` — Scheduler Server integration

- Wired plugin scheduled work into the 20 TPS runtime owner with a fixed maximum of 256 executions per tick.
- Scheduled task failures are logged/recorded but do not prevent the world tick from advancing.
- The first integration commit contained a mechanical `server.go` bracket error caught by the formatting gate; `77983c6` repaired it before any further feature work was stacked.

### `4cae88f` — Command API contract

- Added canonical lower-case command names, definitions/aliases, invocation and synchronous Source/Reply contracts.
- Command sources expose only public player snapshots and reply capability.

### `eb8a6b9` / `477b23b` — owner-scoped command registry

- Added plugin-owned command registrations with conflict detection and host-reserved literals.
- Commands are automatically removed on disable and failed-enable rollback.
- Handler panic/error and per-plugin command timings are isolated/recorded.
- `477b23b` is the pure `gofmt` correction caught by CI before semantic tests were allowed to run.

### `c12242e` — Server command dispatch

- Unknown built-in commands are offered to the plugin command registry on the existing tick-owned command path.
- Core command literals/aliases are reserved before plugins enable.
- Added end-to-end `/alias args...` dispatch testing, public player source validation, replies and command metrics.

### `2277009` — ADR 0002

- Recorded the capability-based API, runtime ownership and loader-independence decisions as an explicit architecture decision.

### `109e316` — synchronous command Source lease

Self-review found that the public Source contract said it was valid only during the synchronous handler, while the first host adapter physically contained server/session pointers that a plugin could retain.

The adapter is now an enforceable lease: once command dispatch returns, retained sources report `SourceUnknown`, `Player()` fails and `Reply()` becomes a no-op. A mutex serializes invalidation against accidental asynchronous plugin calls so the documented lifetime is a real host property rather than a convention.

## Current invariant

All first-generation plugin execution surfaces are runtime-owned:

```text
Server tick owner
  ├── dispatch EventBus handlers
  ├── execute plugin Commands
  └── advance plugin Scheduler
```

Network goroutines never invoke plugin gameplay callbacks directly. Plugins cannot enqueue arbitrary callbacks into the core runtime mailbox.

All extension registrations have an owner and cleanup path. Disable/rollback must leave no event handler, command or scheduled task belonging to that plugin.

## Deliberate non-goals for this branch

- No Go `.so` loader.
- No WASM runtime.
- No plugin manifest/package format.
- No hot reload promise.
- No public mutable World/Player/Entity API.
- No inventory/item API.
- No public block API until stable resource-key/block-state models replace protocol numeric IDs.
- No attempt to make the current literal-only Minecraft command packet a complete Brigadier tree.

These are follow-on capabilities. The next project milestone is **playability**, not API surface expansion: run CyuCore against a real Minecraft 26.2 client through Configuration and Play, then fix the concrete Play-state gaps before adding more plugin features.

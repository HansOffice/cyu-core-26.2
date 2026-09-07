# ADR 0002: Plugin API Is Capability-Based and Runtime-Owned

Status: Accepted

## Context

CyuCore now has explicit tick ownership for player/world state. A plugin API that exposes `Server`, `PlayerSession`, `net.Conn`, protocol packets, internal maps, or a generic cross-thread callback queue would immediately undo that work: plugins could mutate authoritative state from arbitrary goroutines and the public ABI would become coupled to protocol/version implementation details.

The plugin system also needs to remain independent from the eventual loading technology. Built-in Go plugins, a future WASM runtime, or another loader should adapt to one semantic API rather than each loader defining its own lifecycle and threading rules.

## Decision

Plugin API v1 is a host-owned capability container:

- `plugin.Context` exposes only stable public services: Logger, Events, Commands, and Scheduler.
- Public player data is represented by immutable snapshots. Plugins do not receive `PlayerSession` or mutable internal entities.
- Plugins can subscribe to events, register commands, and schedule tasks, but only the CyuCore host can dispatch events, invoke commands, and advance scheduled work.
- Event handlers, command handlers, and scheduled tasks execute synchronously on the server runtime owner. Handlers must not perform blocking disk/network I/O.
- All registrations are scoped to their plugin owner. Disable and failed-enable rollback remove event handlers, commands, and scheduled tasks automatically.
- Plugin panics are converted to host errors with plugin identity/phase information rather than crashing the server process.
- Host diagnostics record lifecycle, event, command, and task timings/errors per plugin.
- Core command literals are reserved before plugins enable, so extension registration cannot shadow host semantics accidentally.

## Scheduler and ADR 0001

The public scheduler accepts a Go handler callback, but that callback is **not** a runtime mailbox work item. It is stored inside pluginruntime's owner-scoped task heap and is invoked only when the tick owner calls `AdvanceTick`.

ADR 0001 still applies unchanged: the cross-thread runtime mailbox carries only typed value events and never arbitrary callbacks. Plugin code has no API that inserts a callback into that mailbox.

## Loader boundary

No dynamic loader is part of API v1. A future Go/WASM loader is an adapter that must:

1. produce a `plugin.Descriptor`;
2. adapt module lifecycle to `Enable`/`Disable` semantics;
3. expose only the same public capabilities;
4. preserve runtime-owner execution and cleanup rules;
5. never redefine Minecraft protocol or server ownership semantics.

This keeps CyuCore version, Minecraft protocol version, Plugin API version, and plugin packaging/runtime version as separate compatibility axes.

## Consequences

The first API is intentionally smaller than Bukkit/Paper-style server-object APIs. Adding a new gameplay mutation surface requires a deliberately designed public service (for example World/BlockState or Inventory APIs) rather than leaking an internal pointer for convenience.

This creates more up-front design work, but it lets internal player/world/protocol implementations change without forcing every plugin to follow those rewrites, and makes sandboxed loaders such as WASM viable later.
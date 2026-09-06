# CyuCore Development Log

This file is an append-only engineering log for decisions and implementation milestones. It complements Git history: commit messages state *what* changed; entries here preserve *why* it changed and what constraints future work must respect.

## 2026-09-06 — Architecture foundation

### Context

The initial prototype successfully exercises a broad vertical slice of Minecraft 26.2: handshake/login/configuration/play states, chunk delivery, player movement, multiplayer visibility, block interaction and commands. Two real client failure reports then exposed incomplete Configuration registry/tag data (`minecraft:damage_type / minecraft:is_fire` and `minecraft:worldgen/world_preset`).

### Decisions

- Treat the existing implementation as a protocol/gameplay prototype to migrate, not as the final package structure.
- Separate network transport, versioned protocol codecs, runtime/tick ownership, registry data, game systems and the public plugin API.
- Introduce a real 20 TPS runtime before adding more gameplay features.
- Replace fake TPS/ping/startup metrics with measurements.
- Replace opaque registry/tag packet dumps with structured, validated Minecraft 26.2 data.
- Never silently drop protocol packets on queue pressure.
- Keep plugin-facing contracts separate from internal server structures; design the API before committing to a dynamic plugin loader.
- Prefer small, buildable commits with tests over a one-shot rewrite.

### Next implementation steps

1. Add a measured tick loop and wire world time to it.
2. Introduce protocol codec primitives with round-trip and malformed-input tests.
3. Make connection/session lifecycle race-safe and define outbound backpressure semantics.
4. Build a structured registry/tag model and validation layer before attempting another Configuration workaround.
5. Establish the first plugin API contracts (lifecycle, events, commands, scheduler) only after runtime ownership is explicit.

## 2026-09-06 — Real tick runtime and metrics

### What changed

- Added `internal/runtime/tick`, a restart-safe measured tick loop with explicit lifecycle.
- Wired the server world clock to a real 20 TPS callback; world time now advances one game tick per server tick instead of advancing 20 ticks from a one-second timer.
- Added observed TPS, last-tick MSPT, EWMA MSPT and total-tick metrics.
- Replaced the hard-coded console TPS/MSPT output with runtime measurements.
- Replaced synthetic startup sleeps and the fixed `0.642s` startup claim with actual elapsed startup time.
- Added unit tests for tick-loop lifecycle and duration metrics.

### Constraints preserved

- The tick package is independent of Minecraft protocol/game objects.
- Networking is still concurrent; this commit does not pretend existing player/session races are solved. Moving authoritative state mutation behind the runtime queue remains a separate migration.
- World-time packets are broadcast once per 20 ticks to avoid unnecessary network traffic while the authoritative world time advances every tick.

## 2026-09-06 — Continuous integration baseline

### What changed

- Added GitHub Actions CI for every `main`, `feat/**` and pull-request change.
- CI rejects non-`gofmt` code, runs all tests, runs `go vet`, and executes the race detector.

### Why

The project is entering architectural migration. A server core cannot rely on manual client joins as its only regression test; each small commit now has an automated build-quality gate before more invasive networking and protocol changes are attempted.

## 2026-09-06 — Protocol codec foundation

### What changed

- Added `internal/protocol` for version-independent wire primitives.
- Implemented bounded VarInt, String and packet framing codecs with explicit malformed/oversized input errors.
- Made packet writes robust to partial `io.Writer` writes instead of assuming one `Write` flushes an entire packet.
- Migrated the prototype's root protocol helpers to compatibility wrappers over the new package, preserving current call sites while moving ownership out of `package main`.
- Added round-trip, malformed VarInt, size-limit and partial-writer tests.

### Why

Binary wire format is a foundational boundary. Future version-specific packet structs can build on one tested codec rather than duplicating byte handling across `buildXXX()` functions. Compatibility wrappers keep this migration small and reversible.

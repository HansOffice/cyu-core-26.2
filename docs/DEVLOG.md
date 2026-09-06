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

## 2026-09-06 — CI bootstrap formatting exception

### What happened

The first CI run failed before compilation because the prototype's `registries.go` blob file was already not `gofmt`-clean. That file is an opaque Base64/GZip bootstrap payload scheduled for replacement by structured registry/tag data.

### Decision

- Keep strict formatting checks for every other Go source file.
- Temporarily exclude exactly `registries.go` from the formatting gate rather than reformatting or normalizing a data blob we intend to delete.
- The exclusion must be removed in the same migration that removes the opaque registry bootstrap.

This is a documented temporary exception, not a general generated-code exemption.

## 2026-09-06 — Session lifecycle and outbound backpressure

### What changed

- Replaced the unsynchronized `SessionState` field with atomic state accessors used by both session and server goroutines.
- Added a dedicated session `done` signal; the outbound channel is no longer closed during shutdown, eliminating the `send on closed channel` race class.
- `SendPacket` now reports success/failure and treats a full outbound queue as a slow-client failure: the session is closed instead of silently discarding a stateful Minecraft packet.
- Writer shutdown now selects on the lifecycle signal rather than ranging over a channel that other goroutines may still target.
- Added lifecycle, queue-pressure and concurrent send/close regression tests intended to run under the CI race detector.

### Known remaining concurrency work

Player transform/gameplay fields (`x/y/z`, rotation, on-ground state, game mode and teleport sequence) are still shared between network and server paths. They are intentionally not hidden by ad-hoc atomics here; the next runtime-ownership migration will move these mutations behind a coherent player-state boundary.

## 2026-09-06 — Tick lifecycle self-review fix

### What changed

- Added an explicit `stopping` lifecycle state to the tick loop.
- `running` remains true until the active tick goroutine has actually exited; a concurrent `Start` can no longer create a second loop during shutdown.
- Concurrent/repeated Stop attempts do not close the stop channel twice.
- Added restart and stop/start overlap regression tests.

### Why

A post-implementation PR self-review found that the first Stop implementation published `running=false` before waiting for the old goroutine to exit. That left a narrow window where another caller could start a second tick loop. The code compiled and CI was green, but the lifecycle invariant was wrong, so it was fixed before merge rather than accepted as a theoretical edge case.

## 2026-09-06 — Structured registry graph foundation

### What changed

- Added `internal/registry` with strict namespaced identifiers, ordered immutable registries, stable derived runtime IDs, resolved tag groups and defensive copies at API boundaries.
- Added a strict JSON dataset decoder for generated/versioned registry data; unknown fields and trailing JSON are rejected rather than silently ignored.
- Added graph validation that rejects duplicate registries/entries/tags and tag references to missing entries.
- Added version-agnostic `Requirements` validation so a protocol version can declare required registries/tags without hard-coding Minecraft 26.2 assumptions into the generic model.
- Added regression tests that explicitly detect the two real Configuration failure classes already observed: missing `minecraft:damage_type/minecraft:is_fire` and missing `minecraft:worldgen/world_preset`.
- Reserved `data/26.2` for reproducible generated vanilla data and documented provenance/validation requirements.

### Deliberate non-goals

- No hand-written or partial 26.2 registry payload was added.
- The existing opaque `registries.go` bootstrap remains in use until a complete authoritative dataset and v776 encoder pass validation and vanilla-client testing.
- This commit does not guess the complete 26.2 required-registry list. That policy belongs to the version-specific data/protocol layer and must be derived from a trustworthy source rather than crash-by-crash patching.

## 2026-09-07 — Java network NBT and typed registry payloads

### What changed

- Added a standalone Java Edition network-NBT value/encoder layer with deterministic compound output, homogeneous-list validation, exact big-endian primitive encoding and Java Modified UTF-8 strings.
- Kept NBT independent of Registry and protocol-v776 packages so the same implementation can later serve block entities, item/component data and persistence boundaries.
- Added a canonical typed-JSON NBT interchange format for generated data. Every numeric value declares its exact NBT type (`byte`, `short`, `int`, `long`, `float`, `double`) instead of relying on JSON number inference.
- Migrated runtime registry entries from opaque `json.RawMessage` payloads to immutable `nbt.Value` trees with defensive deep copies.
- Registry dataset decoding now rejects ambiguous untyped payloads before they can reach the network encoder.

### Why

RegistryData carries optional anonymous NBT. Keeping registry payloads as arbitrary JSON would move type guessing into the wire layer and make values such as `float` versus `double` or `byte` versus `int` depend on decoder accidents. The runtime now owns exact NBT semantics before version-specific packet encoding begins.

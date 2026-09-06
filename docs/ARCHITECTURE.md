# CyuCore Architecture

CyuCore is a native Go implementation of a Minecraft Java Edition server. The project is intended to become a maintainable server platform rather than a collection of packet handlers.

## Design goals

1. **Protocol correctness before feature count.** A client-visible feature is not complete until the corresponding protocol state is valid and testable.
2. **Game state has explicit ownership.** Network I/O may be concurrent; authoritative game-state mutation is committed by the server tick/runtime rather than arbitrary goroutines.
3. **Protocol is not gameplay.** Minecraft protocol versions, registry encodings and packet IDs must not leak into world/entity/gameplay logic.
4. **Data is data.** Registries, tags, blocks, items and other versioned vanilla data are represented as structured data with validation, not opaque packet dumps.
5. **Public API is not internal API.** Plugins depend on a versioned CyuCore API. Internal player/world/network representations may evolve without breaking plugins.
6. **Measure, do not pretend.** TPS, MSPT, ping, memory and startup timings must be measured values. No synthetic performance output.
7. **Backpressure over silent corruption.** Protocol packets must never be silently dropped because a queue is full. Slow clients are backpressured or disconnected deliberately.
8. **Tests at boundaries.** Binary codecs, registry dependency validation, chunk encoding, lifecycle and tick behavior require automated tests.

## Target package layout

The migration is incremental; the current root `package main` remains operational while responsibilities move behind stable boundaries.

```text
cmd/cyu-server/              executable entrypoint (later migration)
internal/network/            connection lifecycle, queues, compression, encryption
internal/protocol/           generic binary codec primitives
internal/protocol/java/v776/ Minecraft 26.2 packet IDs and codecs
internal/runtime/            tick loop, scheduler, ownership boundaries
internal/registry/           structured registries/tags and dependency validation
internal/world/              world, chunk, section, block state, persistence
internal/entity/             entity model and tracking
internal/player/             authoritative player state and gameplay systems
api/                         versioned plugin-facing contracts
plugin/                      plugin manager/runtime adapters
data/26.2/                   versioned vanilla data
```

Dependencies should generally point downward from gameplay into abstractions, never from internal game logic into a concrete Minecraft protocol version.

## Runtime ownership model

```text
connection goroutines
        |
        | decoded intents
        v
  inbound event queue
        |
        v
   20 TPS runtime  ----> scheduler
        |
        +---- players
        +---- worlds/chunks
        +---- entities
        +---- plugin events
        |
        v
 outbound messages
        |
        v
 connection writers
```

A connection can decode packets concurrently, but it should not be able to mutate arbitrary world/player state directly. The runtime is the authority for game-state transitions. Expensive pure work (chunk encoding, compression, storage, path calculations) may use workers as long as the result is committed through the owning runtime.

## Protocol model

Three versions are intentionally independent:

- **Minecraft protocol version**: wire compatibility, e.g. protocol 776.
- **CyuCore version**: server implementation release.
- **CyuCore Plugin API version**: compatibility contract for plugins.

Packet codecs should be typed and version-scoped. Raw `buildXXX()` byte builders in the prototype are migration sources, not the long-term API.

## Registry and vanilla data model

Configuration data must form a self-consistent graph. A tag may only reference entries present in the corresponding registry; registry references must resolve before `finish_configuration` is sent.

Opaque Base64/GZip registry packet captures are temporary bootstrap data and must be replaced with structured 26.2 data plus validation. Client crash reports for missing registries/tags are treated as regression cases.

## Plugin architecture

The plugin API is designed before a dynamic loader is chosen.

Initial contracts will cover:

- lifecycle (`Enable`, `Disable`)
- events
- commands
- scheduler
- players/world access through interfaces
- logging/configuration
- permissions
- plugin timing/health metrics

Plugins must not receive internal `net.Conn`, mutable runtime maps, packet queues or concrete internal player/world structures.

The first implementation may use built-in Go plugins for API development. A sandboxed, cross-platform runtime such as WebAssembly is the preferred long-term dynamic-plugin direction; Go's native `plugin` package is not considered a portable primary plugin format.

## Performance policy

Optimization follows measurement. Required instrumentation includes tick duration, observed TPS, queue pressure, packet encode/write costs, chunk generation/encoding costs and per-plugin tick cost.

High-frequency paths should minimize allocations, but pooling is introduced only where benchmarks demonstrate value. Correct ownership and bounded queues come before micro-optimizations.

## Migration rule

Every architectural migration must leave the repository buildable and should avoid large rewrites that mix unrelated concerns. New behavior should gain tests before or during migration. Temporary compatibility wrappers are acceptable when they keep commits small and reversible.

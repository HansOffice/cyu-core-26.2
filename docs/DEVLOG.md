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

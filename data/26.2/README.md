# Minecraft 26.2 vanilla data

This directory contains the **generated, version-pinned vanilla Configuration dataset** embedded into CyuCore for Minecraft 26.2 / protocol 776.

The checked-in files are not hand-maintained compatibility patches:

- `registry-set.json` is produced from an actual offline Minecraft 26.2 vanilla server Configuration session.
- `manifest.json` pins the Minecraft/protocol/data versions, exact official `server.jar` SHA-256, capture command and consumed Mojang report hashes.
- `vanilla_data.go` embeds and validates both files before the server starts accepting players.

Runtime numeric IDs are derived deterministically from vanilla packet/report ordering. Gameplay code must never maintain a second registry-ID table.

## Authoritative generation path

`.github/workflows/vanilla-data.yml` is the reproducible verification path. It:

1. downloads the official Minecraft 26.2 `server.jar`
2. verifies Mojang's published SHA-1 before executing it
3. runs Mojang's Java 25 datagen to obtain `reports/registries.json`
4. starts an isolated offline vanilla server with network compression disabled
5. runs `cmd/vanilla-capture`, a minimal protocol-776 client
6. selects zero known packs so vanilla sends complete synchronized registry NBT
7. captures the exact `RegistryData` sequence and `UpdateTags` payload
8. resolves static tag registry IDs through Mojang's generated registry report
9. rebuilds CyuCore's structured dataset and compares it byte-for-byte with the checked-in files
10. runs the Go test suite against the generated result

Java is a **data-generation/verification dependency only**. A released CyuCore server remains a Go binary and does not invoke Java at runtime.

## Protocol scope

Minecraft 26.2 synchronizes exactly 29 data-driven registries through Configuration `RegistryData`. The checked-in dataset additionally contains the static registries required to resolve numeric IDs in vanilla's network-safe `UpdateTags` groups.

`minecraft:worldgen/world_preset` is intentionally **not** a synchronized RegistryData registry and must not appear as an UpdateTags group. The earlier client failure mentioning that registry was caused by an invalid tag scope, not by a missing RegistryData packet.

`minecraft:damage_type` is synchronized, and the dataset must include the `minecraft:is_fire` tag. This remains a regression anchor for the original Configuration crash.

## Validation requirements

The checked-in dataset must always:

1. decode through `internal/registry`
2. contain no duplicate registries, entries or tags
3. contain no tag references to missing registry entries
4. contain the exact protocol-776 synchronized registry sequence
5. contain `minecraft:damage_type/minecraft:is_fire`
6. exclude `minecraft:worldgen/world_preset` from the synchronized/tag network scope
7. round-trip through CyuCore's v776 RegistryData and UpdateTags encoders
8. reproduce byte-for-byte through the official vanilla capture workflow
9. pass normal tests, `go vet`, and the race detector

The previous Base64/GZip `registries.go` bootstrap was removed when this dataset became the runtime source of truth. There is no fallback opaque packet dump.

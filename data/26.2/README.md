# Minecraft 26.2 vanilla data

This directory is reserved for **generated, version-pinned vanilla data** consumed by CyuCore's structured registry/data layer.

No partial hand-written registry set should be committed here merely to make a client advance further through Configuration. A checked-in dataset must be reproducible and self-consistent.

## Provenance requirements

Every generated dataset added here must document:

- exact Minecraft version (`26.2` for this directory)
- source artifact or upstream data source
- source hash/version when available
- extraction/generation tool and command
- transformations performed by CyuCore tooling
- validation command and result

Runtime numeric IDs must be derived deterministically from the source ordering. They must not be maintained manually in gameplay code.

## Validation requirements

Before a dataset can replace the prototype `registries.go` bootstrap blob it must, at minimum:

1. decode through `internal/registry`
2. contain no duplicate registries, entries or tags
3. contain no tag references to missing registry entries
4. satisfy the complete Minecraft 26.2 Configuration requirement set
5. produce RegistryData/UpdateTags packets accepted by an unmodified 26.2 client
6. retain regression coverage for previously observed missing `minecraft:damage_type/minecraft:is_fire` and `minecraft:worldgen/world_preset` failures

The opaque bootstrap blob remains temporary until those conditions are met; this directory must not become a second collection of ad-hoc compatibility patches.

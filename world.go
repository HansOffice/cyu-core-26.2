package main

type BlockPos struct {
	X, Y, Z int
}

// World is owned by the server tick goroutine after startup. Callers must route
// mutations through the runtime mailbox instead of adding internal locks here.
// Keeping the model single-owner makes later chunk/entity simulation easier to
// reason about and avoids hiding ownership bugs behind sync primitives.
type World struct {
	modifiedBlocks map[BlockPos]int
	worldAge       int64
	timeOfDay      int64
	spawnX         float64
	spawnY         float64
	spawnZ         float64
}

func NewWorld(spawnX, spawnY, spawnZ float64) *World {
	w := &World{
		modifiedBlocks: make(map[BlockPos]int),
		spawnX:         spawnX,
		spawnY:         spawnY,
		spawnZ:         spawnZ,
	}
	w.initSpawnPlatform()
	return w
}

func (w *World) initSpawnPlatform() {
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			pos := BlockPos{X: x, Y: 64, Z: z}
			switch {
			case x == 0 || x == 15 || z == 0 || z == 15:
				w.modifiedBlocks[pos] = BlockGlowstone
			case (x+z)%2 == 0:
				w.modifiedBlocks[pos] = BlockStoneBricks
			default:
				w.modifiedBlocks[pos] = BlockGrass
			}
		}
	}
}

func (w *World) GetBlock(x, y, z int) int {
	pos := BlockPos{X: x, Y: y, Z: z}
	if blockID, ok := w.modifiedBlocks[pos]; ok {
		return blockID
	}
	if y == 64 && x >= 0 && x < 16 && z >= 0 && z < 16 {
		return BlockStoneBricks
	}
	return BlockAir
}

func (w *World) SetBlock(x, y, z int, blockID int) {
	w.modifiedBlocks[BlockPos{X: x, Y: y, Z: z}] = blockID
}

func (w *World) RangeModifiedBlocks(fn func(BlockPos, int) bool) {
	if w == nil || fn == nil {
		return
	}
	for pos, blockID := range w.modifiedBlocks {
		if !fn(pos, blockID) {
			return
		}
	}
}

func (w *World) AdvanceTime(ticks int64) (int64, int64) {
	w.worldAge += ticks
	w.timeOfDay = normalizeTimeOfDay(w.timeOfDay + ticks)
	return w.worldAge, w.timeOfDay
}

func (w *World) SetTimeOfDay(tod int64) {
	w.timeOfDay = normalizeTimeOfDay(tod)
}

func (w *World) Time() (int64, int64) {
	return w.worldAge, w.timeOfDay
}

func normalizeTimeOfDay(tod int64) int64 {
	tod %= 24000
	if tod < 0 {
		tod += 24000
	}
	return tod
}

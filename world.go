package main

import (
	"sync"
	"sync/atomic"
)

type BlockPos struct {
	X, Y, Z int
}

type World struct {
	modifiedBlocks sync.Map
	worldAge       atomic.Int64
	timeOfDay      atomic.Int64
	spawnX         float64
	spawnY         float64
	spawnZ         float64
}

func NewWorld(spawnX, spawnY, spawnZ float64) *World {
	w := &World{
		spawnX: spawnX,
		spawnY: spawnY,
		spawnZ: spawnZ,
	}
	w.initSpawnPlatform()
	return w
}

func (w *World) initSpawnPlatform() {
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			pos := BlockPos{X: x, Y: 64, Z: z}
			if x == 0 || x == 15 || z == 0 || z == 15 {
				w.modifiedBlocks.Store(pos, BlockGlowstone)
			} else if (x+z)%2 == 0 {
				w.modifiedBlocks.Store(pos, BlockStoneBricks)
			} else {
				w.modifiedBlocks.Store(pos, BlockGrass)
			}
		}
	}
}

func (w *World) GetBlock(x, y, z int) int {
	pos := BlockPos{X: x, Y: y, Z: z}
	if val, ok := w.modifiedBlocks.Load(pos); ok {
		return val.(int)
	}
	if y == 64 && x >= 0 && x < 16 && z >= 0 && z < 16 {
		return BlockStoneBricks
	}
	return BlockAir
}

func (w *World) SetBlock(x, y, z int, blockID int) {
	pos := BlockPos{X: x, Y: y, Z: z}
	if blockID == BlockAir {
		w.modifiedBlocks.Store(pos, BlockAir)
	} else {
		w.modifiedBlocks.Store(pos, blockID)
	}
}

func (w *World) AdvanceTime(ticks int64) (int64, int64) {
	age := w.worldAge.Add(ticks)
	tod := (w.timeOfDay.Add(ticks)) % 24000
	return age, tod
}

func (w *World) SetTimeOfDay(tod int64) {
	w.timeOfDay.Store(tod % 24000)
}

package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func testGameMap() *GameMap {
	return &GameMap{
		Width: 3, Height: 3,
		Tiles: [][]TileType{
			{TileWall, TileWall, TileBrick},
			{TileWall, TileEmpty, TileWall},
			{TileWall, TileWall, TileWall},
		},
	}
}

func TestIsSolid(t *testing.T) {
	m := testGameMap()

	assert.False(t, m.IsSolid(1, 1), "empty tile")
	assert.True(t, m.IsSolid(0, 0), "wall tile")
	assert.True(t, m.IsSolid(2, 0), "brick tile")
	assert.True(t, m.IsSolid(-1, 0), "out-of-bounds left")
	assert.True(t, m.IsSolid(3, 0), "out-of-bounds right")
	assert.True(t, m.IsSolid(0, -1), "out-of-bounds top")
	assert.True(t, m.IsSolid(0, 3), "out-of-bounds bottom")
}

func TestIsWalkable(t *testing.T) {
	m := testGameMap()

	assert.True(t, m.IsWalkable(1, 1), "empty tile")
	assert.False(t, m.IsWalkable(0, 0), "wall tile")
	assert.False(t, m.IsWalkable(-1, 0), "out-of-bounds")
	assert.False(t, m.IsWalkable(3, 3), "out-of-bounds")
}

func TestTileAt(t *testing.T) {
	m := testGameMap()

	assert.Equal(t, TileEmpty, m.TileAt(1, 1))
	assert.Equal(t, TileWall, m.TileAt(0, 0))
	assert.Equal(t, TileWall, m.TileAt(-1, 0), "out-of-bounds returns TileWall")
}

func TestDestroyBrick(t *testing.T) {
	m := testGameMap()

	assert.True(t, m.IsSolid(2, 0), "brick is solid before destroy")
	assert.True(t, m.DestroyBrick(2, 0))
	assert.Equal(t, TileEmpty, m.Tiles[0][2])
	assert.False(t, m.IsSolid(2, 0), "tile not solid after destroy")
	assert.False(t, m.DestroyBrick(2, 0), "double destroy returns false")
}

func TestDestroyBrick_Wall(t *testing.T) {
	m := testGameMap()
	assert.False(t, m.DestroyBrick(0, 0), "cannot destroy permanent wall")
	assert.Equal(t, TileWall, m.Tiles[0][0], "wall tile unchanged")
}

func TestDestroyBrick_OOB(t *testing.T) {
	m := testGameMap()
	assert.False(t, m.DestroyBrick(-1, 0))
}

package game

import "github.com/Mark-Grigorev/GoTanks/internal/loader"

type TileType int

const (
	TileEmpty TileType = 0
	TileWall  TileType = 1
	TileBrick TileType = 2
)

type GameMap struct {
	ID     string
	Width  int
	Height int
	Tiles  [][]TileType
	Spawns [][2]int
}

func NewGameMap(cfg *loader.MapConfig) *GameMap {
	tiles := make([][]TileType, cfg.Height)
	for y, row := range cfg.Tiles {
		tiles[y] = make([]TileType, cfg.Width)
		for x, t := range row {
			if x < cfg.Width {
				tiles[y][x] = TileType(t)
			}
		}
	}
	spawns := make([][2]int, len(cfg.Spawns))
	copy(spawns, cfg.Spawns)
	return &GameMap{
		ID:     cfg.ID,
		Width:  cfg.Width,
		Height: cfg.Height,
		Tiles:  tiles,
		Spawns: spawns,
	}
}

func (m *GameMap) IsWalkable(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false
	}
	return m.Tiles[y][x] == TileEmpty
}

func (m *GameMap) IsSolid(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return true
	}
	t := m.Tiles[y][x]
	return t == TileWall || t == TileBrick
}

func (m *GameMap) TileAt(x, y int) TileType {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return TileWall
	}
	return m.Tiles[y][x]
}

func (m *GameMap) DestroyBrick(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false
	}
	if m.Tiles[y][x] == TileBrick {
		m.Tiles[y][x] = TileEmpty
		return true
	}
	return false
}

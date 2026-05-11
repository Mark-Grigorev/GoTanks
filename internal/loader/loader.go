package loader

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed tanks maps
var embeddedFS embed.FS

type TankConfig struct {
	ID            string `yaml:"id"             json:"id"`
	Name          string `yaml:"name"           json:"name"`
	Speed         int    `yaml:"speed"          json:"speed"`
	HP            int    `yaml:"hp"             json:"hp"`
	BulletSpeed   int    `yaml:"bullet_speed"   json:"bullet_speed"`
	BulletDamage  int    `yaml:"bullet_damage"  json:"bullet_damage"`
	ShootCooldown int    `yaml:"shoot_cooldown" json:"shoot_cooldown"`
	Hull          string `yaml:"hull"           json:"hull"`
	Gun           string `yaml:"gun"            json:"gun"`
}

type MapConfig struct {
	ID     string   `yaml:"id"     json:"id"`
	Name   string   `yaml:"name"   json:"name"`
	Width  int      `yaml:"width"  json:"width"`
	Height int      `yaml:"height" json:"height"`
	Tiles  [][]int  `yaml:"tiles"  json:"tiles"`
	Spawns [][2]int `yaml:"spawns" json:"spawns"`
}

type Loader struct {
	Tanks map[string]*TankConfig
	Maps  map[string]*MapConfig
}

// Load reads tank and map configs from the embedded filesystem.
func Load() (*Loader, error) {
	return LoadFrom(embeddedFS, "tanks", "maps")
}

// LoadFrom reads configs from any fs.FS — used in tests.
func LoadFrom(fsys fs.FS, tanksDir, mapsDir string) (*Loader, error) {
	l := &Loader{
		Tanks: make(map[string]*TankConfig),
		Maps:  make(map[string]*MapConfig),
	}
	if err := loadDir(fsys, tanksDir, func(data []byte) error {
		var t TankConfig
		if err := yaml.Unmarshal(data, &t); err != nil {
			return err
		}
		l.Tanks[t.ID] = &t
		return nil
	}); err != nil {
		return nil, fmt.Errorf("tanks: %w", err)
	}
	if err := loadDir(fsys, mapsDir, func(data []byte) error {
		var m MapConfig
		if err := yaml.Unmarshal(data, &m); err != nil {
			return err
		}
		l.Maps[m.ID] = &m
		return nil
	}); err != nil {
		return nil, fmt.Errorf("maps: %w", err)
	}
	return l, nil
}

func loadDir(fsys fs.FS, dir string, fn func([]byte) error) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return err
		}
		if err := fn(data); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
	}
	return nil
}

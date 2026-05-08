package loader

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type TankConfig struct {
	ID            string `yaml:"id"             json:"id"`
	Name          string `yaml:"name"           json:"name"`
	Speed         int    `yaml:"speed"          json:"speed"`
	HP            int    `yaml:"hp"             json:"hp"`
	BulletSpeed   int    `yaml:"bullet_speed"   json:"bullet_speed"`
	BulletDamage  int    `yaml:"bullet_damage"  json:"bullet_damage"`
	ShootCooldown int    `yaml:"shoot_cooldown" json:"shoot_cooldown"`
	Hull          string `yaml:"hull"           json:"hull"` // path inside tanks-sprites/PNG/
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

func Load(tanksDir, mapsDir string) (*Loader, error) {
	l := &Loader{
		Tanks: make(map[string]*TankConfig),
		Maps:  make(map[string]*MapConfig),
	}
	if err := loadDir(tanksDir, func(data []byte) error {
		var t TankConfig
		if err := yaml.Unmarshal(data, &t); err != nil {
			return err
		}
		l.Tanks[t.ID] = &t
		return nil
	}); err != nil {
		return nil, fmt.Errorf("tanks: %w", err)
	}
	if err := loadDir(mapsDir, func(data []byte) error {
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

func loadDir(dir string, fn func([]byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if err := fn(data); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
	}
	return nil
}

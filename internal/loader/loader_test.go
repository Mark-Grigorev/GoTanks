package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tankYAML = `
id: test_tank
name: Test Tank
speed: 5
hp: 3
bullet_speed: 10
bullet_damage: 1
shoot_cooldown: 5
hull: hull.png
gun: gun.png
`

const mapYAML = `
id: test_map
name: Test Map
width: 3
height: 3
tiles:
  - [1, 1, 1]
  - [1, 0, 1]
  - [1, 1, 1]
spawns:
  - [1, 1]
`

func makeTestDirs(t *testing.T) (root, tanksDir, mapsDir string) {
	t.Helper()
	root = t.TempDir()
	tanksDir = filepath.Join(root, "tanks")
	mapsDir = filepath.Join(root, "maps")
	require.NoError(t, os.Mkdir(tanksDir, 0755))
	require.NoError(t, os.Mkdir(mapsDir, 0755))
	return
}

func TestLoad_HappyPath(t *testing.T) {
	root, tanksDir, mapsDir := makeTestDirs(t)
	os.WriteFile(filepath.Join(tanksDir, "test_tank.yaml"), []byte(tankYAML), 0644)
	os.WriteFile(filepath.Join(mapsDir, "test_map.yaml"), []byte(mapYAML), 0644)

	l, err := LoadFrom(os.DirFS(root), "tanks", "maps")
	require.NoError(t, err)

	tank, ok := l.Tanks["test_tank"]
	require.True(t, ok, "expected tank 'test_tank' to be loaded")
	assert.Equal(t, 3, tank.HP)
	assert.Equal(t, 5, tank.Speed)

	m, ok := l.Maps["test_map"]
	require.True(t, ok, "expected map 'test_map' to be loaded")
	assert.Equal(t, 3, m.Width)
	assert.Equal(t, 3, m.Height)
	assert.Len(t, m.Spawns, 1)
}

func TestLoad_EmptyDirs(t *testing.T) {
	root, _, _ := makeTestDirs(t)

	l, err := LoadFrom(os.DirFS(root), "tanks", "maps")
	require.NoError(t, err)
	assert.Empty(t, l.Tanks)
	assert.Empty(t, l.Maps)
}

func TestLoad_InvalidYAML(t *testing.T) {
	root, tanksDir, _ := makeTestDirs(t)
	os.WriteFile(filepath.Join(tanksDir, "bad.yaml"), []byte("key: {unclosed\n"), 0644)

	_, err := LoadFrom(os.DirFS(root), "tanks", "maps")
	assert.Error(t, err)
}

func TestLoad_NonYAMLFilesIgnored(t *testing.T) {
	root, tanksDir, _ := makeTestDirs(t)
	os.WriteFile(filepath.Join(tanksDir, "readme.txt"), []byte("not yaml"), 0644)
	os.WriteFile(filepath.Join(tanksDir, "test_tank.yaml"), []byte(tankYAML), 0644)

	l, err := LoadFrom(os.DirFS(root), "tanks", "maps")
	require.NoError(t, err)
	assert.Len(t, l.Tanks, 1)
}

func TestLoad_MissingDir(t *testing.T) {
	_, err := LoadFrom(os.DirFS("/nonexistent"), "tanks", "maps")
	assert.Error(t, err)
}

func TestLoad_Embedded(t *testing.T) {
	l, err := Load()
	require.NoError(t, err)
	assert.NotEmpty(t, l.Tanks)
	assert.NotEmpty(t, l.Maps)
}

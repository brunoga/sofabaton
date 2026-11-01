// Package templates holds the device templates we maintain (see
// profile.FromTemplate), by transport and maker: ir/denon/avr-x8500h.yaml
// is the template named "ir/denon/avr-x8500h". Transports are ir,
// bluetooth and network.
package templates

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

//go:embed ir
var files embed.FS

// FS is the templates, as their YAML files.
var FS fs.FS = files

// Names lists the templates' names, sorted.
func Names() []string {
	var names []string
	_ = fs.WalkDir(FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".yaml" {
			names = append(names, strings.TrimSuffix(p, ".yaml"))
		}
		return err
	})
	return names
}

// Read returns the template of that name.
func Read(name string) ([]byte, error) {
	return fs.ReadFile(FS, strings.TrimSuffix(name, ".yaml")+".yaml")
}

package profile

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// A template is a device's profile as we maintain it, for any hub: the
// profile's fields but the hub's id and the device's name, headed by a
// template key naming the model. It is not applied as it is: FromTemplate makes a profile of it,
// which the user then edits (names, keys, macros) and applies.
//
//	# Denon AVR-X8500H receiver, from Denon's IR command chart
//	template: Denon AVR-X8500H
//	brand: Denon
//	ir: {protocol: DenonK, device: 4, subdevice: 1}
//	commands:
//	  ...

// IsTemplate reports whether data is a template: a mapping with a
// template key.
func IsTemplate(data []byte) bool {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return false
	}
	_, ok := top["template"]
	return ok
}

var (
	templateLine = regexp.MustCompile(`(?m)^template:.*$`)
	deviceLine   = regexp.MustCompile(`(?m)^device:.*$`)
	idLine       = regexp.MustCompile(`(?m)^id:`)
)

// FromTemplate returns a profile made from a template, its comments
// kept: its template line becomes from, where the template came from (a
// name of the templates package, say), followed by the device's name:
// name, or the template's model when name is empty.
func FromTemplate(tmpl []byte, from, name string) ([]byte, error) {
	var top struct {
		Template string `yaml:"template"`
	}
	if err := yaml.Unmarshal(tmpl, &top); err != nil || top.Template == "" {
		return nil, errors.New("profile: not a template (no template key)")
	}
	if len(templateLine.FindAll(tmpl, -1)) != 1 {
		return nil, errors.New("profile: template: the template key must be on a line of its own")
	}
	if idLine.Match(tmpl) || deviceLine.Match(tmpl) {
		return nil, errors.New("profile: a template has no id or device name")
	}
	if name == "" {
		name = top.Template
	}
	head := append(append(scalarLine("from", from), '\n'), scalarLine("device", name)...)
	out := templateLine.ReplaceAllLiteral(tmpl, head)
	if _, err := Load(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("profile: template: %w", err)
	}
	return out, nil
}

// LoadTemplate reads and checks a template.
func LoadTemplate(data []byte) (*Device, error) {
	out, err := FromTemplate(data, "", "")
	if err != nil {
		return nil, err
	}
	return Load(bytes.NewReader(out))
}

// scalarLine is "key: value", the value quoted as YAML needs.
func scalarLine(key, value string) []byte {
	if value == "" {
		return []byte(key + ":")
	}
	b, _ := yaml.Marshal(value)
	return append([]byte(key+": "), bytes.TrimRight(b, "\n")...)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

// Source is the kind of place a value came from, highest precedence first.
type Source string

// The sources, in the order Load consults them.
const (
	SourceFlag       Source = "flag"
	SourceEnv        Source = "env"
	SourceEnvFile    Source = "env_file"
	SourceConfigFile Source = "config_file"
	SourceBaseFile   Source = "base_file"
	SourceDatabase   Source = "database"
	SourceDefault    Source = "default"
	SourceUnset      Source = "unset"
)

// Origin says where a value came from. Name is the flag (with dashes), the env
// name, or the YAML key. File is the file that held it: a .env or YAML file,
// or the target of a ${file:} secret.
type Origin struct {
	Source Source
	Name   string
	File   string
}

// FromBoot is true when an operator set the value outside the product.
// A stored setting with such an origin is read-only in the UI and API.
func (o Origin) FromBoot() bool {
	switch o.Source {
	case SourceFlag, SourceEnv, SourceEnvFile, SourceConfigFile, SourceBaseFile:
		return true
	}
	return false
}

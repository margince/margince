// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"flag"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// laneSettings are the boot settings a debug lane shares with the worker. Its
// flag set also holds the lane's own flags, which Load does not know.
type laneSettings struct {
	reg   *config.Registry
	given *config.CommandFlags
}

// The stored settings join the lane's own, so what a boot source sets is
// locked in a lane as in the worker.
func addLaneSettings(fs *flag.FlagSet, settings ...config.Any) (*laneSettings, error) {
	reg, err := config.NewRegistry(settings, compose.StoredSettings())
	if err != nil {
		return nil, err
	}
	return &laneSettings{reg: reg, given: config.AddFlags(fs, reg)}, nil
}

// load resolves the settings after the flag set parsed, so a flag still beats
// the environment and the files, as at boot. It hands compose the result, as
// the worker's own load does.
func (l *laneSettings) load() (*config.Config, error) {
	loaded, err := config.Load(l.reg, config.Sources{
		Args:           l.given.Args(),
		Environ:        config.Environ(),
		Known:          compose.AllSettingNames(),
		BootstrapRoots: deployconfig.Roots(),
	})
	if err != nil {
		return nil, err
	}
	compose.UseConfig(loaded)
	return loaded, nil
}

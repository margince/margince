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

func addLaneSettings(fs *flag.FlagSet, settings ...config.Any) (*laneSettings, error) {
	reg, err := config.NewRegistry(settings)
	if err != nil {
		return nil, err
	}
	return &laneSettings{reg: reg, given: config.AddFlags(fs, reg)}, nil
}

// load resolves the settings after the flag set parsed, so a flag still beats
// the environment and the files, as at boot.
func (l *laneSettings) load() (*config.Config, error) {
	return config.Load(l.reg, config.Sources{
		Args:           l.given.Args(),
		Environ:        config.Environ(),
		KnownKey:       compose.AllSettingKeys,
		KnownEnv:       compose.AllSettingEnvs,
		BootstrapRoots: deployconfig.Sections(),
	})
}

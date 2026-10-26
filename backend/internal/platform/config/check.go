// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"io"
)

// CheckRequested is whether args ask for --check-config. A command line that
// does not parse asks for nothing: the boot that follows refuses it with the
// fault a check would print.
func CheckRequested(reg *Registry, args []string) bool {
	flags, err := parseFlags(reg, args)
	return err == nil && flags.check
}

// Check loads as a boot does and writes to w the trace, the variables no
// program reads and the settings only another program reads. It returns every
// fault, or nil when the configuration holds. It opens no database, so a
// stored setting with no boot source stays unset.
func Check(w io.Writer, reg *Registry, src Sources) error {
	cfg, faults := loadSources(reg, src)
	if cfg != nil {
		lines := cfg.TraceLines()
		for _, name := range cfg.warnings {
			lines = append(lines, "config: set but read by no Margince program: "+name)
		}
		for _, name := range cfg.elsewhere {
			lines = append(lines, "config: not checked here, another program reads it: "+name)
		}
		for _, line := range lines {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return errors.Join(faults, err)
			}
		}
	}
	if faults != nil {
		return faults
	}
	verdict := "config: valid"
	if cfg != nil && len(cfg.elsewhere) > 0 {
		verdict += fmt.Sprintf(" for this program; the %d setting(s) another program reads need its own --check-config", len(cfg.elsewhere))
	}
	_, err := fmt.Fprintln(w, verdict)
	return err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The boot preflights for what a unit declares it can DO with the outside
// world: land records, run on a clock, and transmit messages.
//
// Split from extensions.go because they share a shape the other preflights do
// not: each validates through the SAME published Validate the manifest
// generator runs, so what a generated manifest accepted cannot be refused at
// boot (or worse, the reverse). Grouped rather than scattered so the next
// capability added has an obvious home.

import (
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// preflightIngress validates one unit's declared ingress sources through the
// same published IngressSource.Validate the manifest generator runs, and
// rejects the same system declared twice.
//
// A duplicate is not harmless here the way a duplicate description would be:
// the system key is half of every landed record's natural key, so two entries
// naming one system are two declarations an operator resolves separately about
// one provenance namespace — and if they ever disagree about Lands, which of
// them the port answered from would be declaration order.
func preflightIngress(e extension.Extension) error {
	seen := make(map[string]bool, len(e.Ingress))
	for _, source := range e.Ingress {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[source.System] {
			return fmt.Errorf("compose: extension %q declares ingress source %q twice", e.Name, source.System)
		}
		seen[source.System] = true
	}
	return nil
}

// preflightJobs validates one unit's scheduled jobs through the same published
// Job.Validate the manifest generator runs, and rejects a job name declared
// twice within the unit — the same fail-closed boundary preflightTools holds,
// for a declaration that reached the composed set outside the generator path.
func preflightJobs(e extension.Extension) error {
	seen := make(map[string]bool, len(e.Jobs))
	for _, job := range e.Jobs {
		if err := job.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[job.Name] {
			return fmt.Errorf("compose: extension %q declares job %q twice", e.Name, job.Name)
		}
		seen[job.Name] = true
	}
	return nil
}

// preflightChannels validates one unit's declared channels, and holds the two
// rules a single declaration cannot check for itself (DESIGN-SP5 §9.1).
//
// Channel.Validate already covers the provider grammar and the Send/Live
// pairing. What is added here is unit-vs-unit COLLISION, which is a fact about
// the composed set rather than about one declaration.
//
// The collision against a CORE connector is the sharper one — a unit shadowing
// `telegram` would have every Telegram reply transmitted on the unit's
// credential instead of the workspace's bot, a silent change of who is sending
// rather than a visible failure — and it is held in the reconcile, which is
// where the core's own transport set is known. Asking here would answer from a
// registry that may not be built yet, and pass the collision it exists to catch.
//
// Both refuse at BOOT rather than at the send, because the send is the moment
// the mistake becomes a message somebody receives.
func preflightChannels(e extension.Extension, claimed map[string]extension.Name) error {
	seen := make(map[string]bool, len(e.Channels))
	for _, ch := range e.Channels {
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[ch.Provider] {
			return fmt.Errorf("compose: extension %q declares channel %q twice", e.Name, ch.Provider)
		}
		seen[ch.Provider] = true
		if other, taken := claimed[ch.Provider]; taken {
			return fmt.Errorf("compose: extension %q declares channel %q, already claimed by extension %q — one provider names one transport",
				e.Name, ch.Provider, other)
		}
		claimed[ch.Provider] = e.Name
	}
	return nil
}

// preflightEntries validates one unit's entries through the published
// Entry.Validate the generator runs, and refuses two entries that open one
// view from one slot.
func preflightEntries(e extension.Extension) error {
	seen := make(map[string]bool, len(e.Entries))
	for _, entry := range e.Entries {
		if err := entry.Validate(e.Name); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		key := string(entry.Slot) + "\x00" + entry.Target
		if seen[key] {
			return fmt.Errorf("compose: extension %q declares two entries in slot %q with target %q", e.Name, entry.Slot, entry.Target)
		}
		seen[key] = true
	}
	return nil
}

// preflightSurfaces checks what a unit draws on core screens: its entries and
// its panels.
func preflightSurfaces(e extension.Extension) error {
	if err := preflightEntries(e); err != nil {
		return err
	}
	return preflightPanels(e)
}

// preflightPanels validates one unit's panels through the published
// Panel.Validate the generator runs. It refuses two panels in one slot, since
// the page draws a unit once per slot. The generator refuses the same at make
// gen; this holds a hand-built registry at boot, as preflightEntries does.
func preflightPanels(e extension.Extension) error {
	seen := make(map[extension.PanelSlot]bool, len(e.Panels))
	for _, panel := range e.Panels {
		if err := panel.Validate(e.Name); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[panel.Slot] {
			return fmt.Errorf("compose: extension %q declares two panels in slot %q", e.Name, panel.Slot)
		}
		seen[panel.Slot] = true
	}
	return nil
}

// preflightEntryObjects refuses an entry or a panel whose object none of its
// unit's operations gates on. No grant of such an object lets a reader use the
// unit.
func preflightEntryObjects(exts []extension.Extension, verbs []extension.Verb) error {
	gated := map[extension.Name]map[string]bool{}
	for _, v := range verbs {
		if gated[v.Unit] == nil {
			gated[v.Unit] = map[string]bool{}
		}
		gated[v.Unit][v.RbacObject] = true
	}
	for _, e := range exts {
		for _, entry := range e.Entries {
			if !gated[e.Name][entry.Object] {
				return fmt.Errorf("compose: extension %q: its entry in slot %q is gated on %q, which none of the unit's operations gates on", e.Name, entry.Slot, entry.Object)
			}
		}
		for _, panel := range e.Panels {
			if !gated[e.Name][panel.Object] {
				return fmt.Errorf("compose: extension %q: its panel in slot %q is gated on %q, which none of the unit's operations gates on", e.Name, panel.Slot, panel.Object)
			}
		}
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ExtensionDescriptor, extensions } from "@composition/extensions";
import type { ComponentType } from "react";
import {
  composedPanels,
  type ExtensionPanelProps,
  type ExtensionPanelRegistry,
} from "../lib/extensions";

// A unit's panels: its own components, drawn inline where a core page offers a
// slot. The unit declares the slot and the RBAC object; the page decides where
// the slot sits and fences each panel off.

/**
 * Every slot a core page draws. Mirrors extension.PanelSlots() in
 * backend/pkg/extension/panel.go; backend/gates/frontendentryslots_test.go
 * fails when the two lists differ.
 */
export const EXTENSION_PANEL_SLOTS = [
  "record_panel.contact",
  "record_panel.company",
  "record_panel.deal",
] as const;

export type ExtensionPanelSlot = (typeof EXTENSION_PANEL_SLOTS)[number];

/** One panel to draw: the unit it belongs to, its grant and its component. */
export type UnitPanel = Readonly<{
  unit: string;
  object: string;
  Component: ComponentType<ExtensionPanelProps>;
}>;

/**
 * Every composed unit's panel for `slot`, in registry order. A declared panel
 * whose component the registry lacks is left out rather than drawn as a hole.
 */
export function panelsForSlot(
  slot: ExtensionPanelSlot,
  registry: readonly ExtensionDescriptor[] = extensions,
  components: ExtensionPanelRegistry = composedPanels,
): readonly UnitPanel[] {
  return registry.flatMap((unit) => {
    const declared = unit.panels.find((panel) => panel.slot === slot);
    // Object.hasOwn, not a bare index: a unit named `constructor` would
    // otherwise read Object itself off the prototype chain.
    const byUnit = Object.hasOwn(components, unit.name)
      ? components[unit.name]
      : undefined;
    const Component =
      byUnit && Object.hasOwn(byUnit, slot) ? byUnit[slot] : undefined;
    return declared && Component
      ? [{ unit: unit.name, object: declared.object, Component }]
      : [];
  });
}

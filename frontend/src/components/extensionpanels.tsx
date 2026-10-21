// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ExtensionDescriptor } from "@composition/extensions";
import { CardBoundary } from "../design-system/cardboundary";
import { grants } from "../lib/capability";
import type { EntityKind } from "../lib/entity";
import { type ExtensionPanelSlot, panelsForSlot } from "../lib/extensionpanels";
import {
  type ExtensionPanelRegistry,
  isExtensionRbacObject,
} from "../lib/extensions";
import { useMe } from "../lib/session";

/**
 * The composed units' panels for one slot, each drawn with the record the page
 * shows. A panel shows only to a reader holding read on its object. A slot with
 * nothing to show draws nothing.
 *
 * Each panel sits in its own CardBoundary, so a unit's render failure stays in
 * that unit's card. The page and the other units' panels never learn of it.
 */
export function ExtensionPanels({
  slot,
  record,
  registry,
  components,
}: Readonly<{
  slot: ExtensionPanelSlot;
  record: Readonly<{ type: EntityKind; id: string }>;
  registry?: readonly ExtensionDescriptor[];
  components?: ExtensionPanelRegistry;
}>) {
  const me = useMe();
  const shown = panelsForSlot(slot, registry, components).filter(
    (panel) =>
      isExtensionRbacObject(panel.object) &&
      grants(me.data, panel.object, "read"),
  );
  if (shown.length === 0) {
    return null;
  }
  // Keyed on the record too, so a panel that failed on one deal starts afresh
  // on the next, with none of the first deal's state.
  return shown.map(({ unit, Component }) => (
    <div key={`${unit}:${record.id}`} data-ext-panel={unit}>
      <CardBoundary>
        <Component record={record} />
      </CardBoundary>
    </div>
  ));
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Switch } from "../design-system/switch";
import { setEdgeLightShown, useEdgeLightShown } from "./agent-edge-preference";
import { LABELS } from "./agentrail-copy";

/**
 * The panel's foot: whether the window's own margins light while the agent
 * works.
 *
 * It belongs to the agent rather than to the settings screen because it is a
 * reading of the agent, and the panel is where a person is already looking at
 * that reading — a preference about a surface you can see from here is one you
 * should be able to answer from here. It is also the only control on this
 * panel, which is why the foot carries nothing else: everything above it
 * REPORTS, and a second verb down here would blur what the surface is for.
 *
 * The margin is decoration with a job (`agent-edge.tsx`), and turning it off
 * costs a reader nothing they cannot get in words: the block above says what
 * the agent is doing, in a sentence, whether or not the window is lit. So this
 * needs no warning and no confirmation — it is a preference, not a trade.
 *
 * A `Switch` rather than a `Checkbox` because flipping it IS the write: the
 * margins go dark on the flip and the browser remembers, with no Save anywhere
 * on this surface to press.
 */
export function EdgeLightSetting() {
  const shown = useEdgeLightShown();
  return (
    <div className="arfoot">
      <Switch
        label={LABELS.edgeLight}
        checked={shown}
        onChange={setEdgeLightShown}
      />
    </div>
  );
}

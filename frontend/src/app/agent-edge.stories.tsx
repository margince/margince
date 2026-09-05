// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { AgentEdge } from "./agent-edge";
import {
  type AgentEdgeReading,
  clearAgentEdge,
  publishAgentEdge,
} from "./agent-edge-signal";

/**
 * The agent's margins, which are the hardest thing on this surface to review from
 * a screenshot: one state is motion and the other is the absence of it. These
 * stories exist so the difference can be seen side by side.
 *
 * Full-screen layout, always. The marks are `fixed` to the viewport, so in a
 * padded canvas they would hug the frame of the story rather than the frame of the
 * window and the widths would read wrong.
 */
const meta: Meta<typeof AgentEdge> = {
  title: "Shell/Agent margins",
  component: AgentEdge,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof AgentEdge>;

/**
 * Publishes a reading for as long as the story is open.
 *
 * The component reads a module-level signal rather than props, because the rail is
 * what knows the agent's state and a second consumer cannot re-derive it
 * (agent-edge-signal.ts). So a story has to say what the agent is doing the same
 * way the rail does, and clear it on the way out or the next story inherits it.
 */
function Edge({ reading, register }: AgentEdgeReading) {
  useEffect(() => {
    publishAgentEdge({ reading, register });
    return clearAgentEdge;
  }, [reading, register]);
  return (
    <>
      <AgentEdge />
      {/* Something behind them, so the marks are read against a page rather than
          against nothing. No wrapper element: the marks are `fixed`, so they need
          no positioned parent and a padded one would only mislead about where
          they sit. */}
      <p>
        The page underneath. The margins take no pointer and are hidden from a
        screen reader, because everything they report is also written in words
        in the rail.
      </p>
    </>
  );
}

/** Nothing at all, which is the resting state and most of the working day. */
export const Still: Story = {
  render: () => <Edge reading={false} register="agent" />,
};

/**
 * The agent's own work in flight: a run, or a call this tab holds open. The
 * whole rim is lit, the light billows in place, and one soft arc laps the
 * perimeter. Movement here means something is happening, and this is the louder
 * of the two registers it can happen in.
 */
export const Reading: Story = {
  render: () => <Edge reading={true} register="agent" />,
};

/**
 * A mailbox import in flight, and nothing else. The same rim in the thin
 * register: about half the thickness, half the breath, a faint head still making
 * its lap. Open this beside `Reading` — the difference is the whole design, and
 * it should be unmistakable at a glance and unremarkable over an afternoon.
 */
export const Importing: Story = {
  render: () => <Edge reading={true} register="capture" />,
};

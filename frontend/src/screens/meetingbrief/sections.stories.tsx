// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../story-utils";
import { briefModel, briefReady } from "./fixtures";
import { Background, BodyPanels, GlanceLine, GoalPanel } from "./sections";
import "./meetingbrief.css";

// The brief's nine sections, out of the drawer.
//
// The connected drawer's frames (Records/Person record/Meeting brief) show these
// inside a Modal, portalled to document.body — which is a fine way to see the
// drawer and a poor way to see the PANELS: every capture there is the whole
// sheet at drawer width. These render the same components inline, which is also
// what makes them the direct story coverage the render gate asks of each file.
//
// The one thing worth reading across the two frames is the TINT. A brief the
// model wrote is indigo throughout and says so once, on the lead; the
// deterministic composition over the same records takes the accent on its lead
// and nothing on the sections, because indigo is a claim about who wrote the
// words and a composition would be borrowing it.

const meta: Meta<typeof GoalPanel> = {
  title: "Records/Person record/Meeting brief/Sections",
  component: GoalPanel,
};
export default meta;
type Story = StoryObj<typeof GoalPanel>;

function sections(brief: typeof briefReady) {
  return () => {
    // Nothing here fetches — the sections take their brief as a prop. The stub
    // is installed so a stray read cannot leave the iframe and resolve to
    // something that looks like an answer.
    installFetchStub({});
    return (
      <StoryProviders>
        <div className="mb-stack" style={{ maxWidth: 665 }}>
          <GlanceLine brief={brief} onOpenRecord={() => {}} />
          <GoalPanel brief={brief} onOpenRecord={() => {}} />
          <BodyPanels brief={brief} onOpenRecord={() => {}} />
          <Background brief={brief} onOpenRecord={() => {}} />
        </div>
      </StoryProviders>
    );
  };
}

/** Assembled without a model: the lead asks for a move in the accent, and the
 *  five sections under it carry no tint at all. */
export const Composed: Story = { render: sections(briefReady) };

/** The model's own brief: indigo on the lead and on every section under it, and
 *  the disclosure badge said once, in the lead's head. */
export const ModelWritten: Story = { render: sections(briefModel) };

/** The model's brief in the dark theme, where `--aiText` over `--aiLight` is
 *  the pair the dark accent lift moves first. */
export const ModelWrittenDark: Story = {
  globals: { theme: "dark" },
  render: sections(briefModel),
};

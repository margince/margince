// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect } from "react";
import { StoryProviders } from "../screens/story-utils";
import { setEdgeLightShown } from "./agent-edge-preference";
import { EdgeLightSetting } from "./agentrail-edgelight";
// The row draws itself as the agent panel's foot (`.arfoot`), and that rule
// lives in the rail's own sheet, which only `agentrail.tsx` imports. A story
// that mounted the row alone would review an unstyled switch.
import "./agentrail.css";

// The agent panel's one control: whether the window's own margins light while
// the agent works.
//
// It has exactly two states and no reads behind it — the answer is a browser
// preference (`agent-edge-preference.ts`), not a fact the installation
// answers — so the two stories below are the whole surface.

/**
 * Put the preference back on the way out.
 *
 * It is ONE answer per browser, and the panel's own story (`Shell/Agent rail`)
 * draws this same switch: a story that left it off would turn the margins off
 * for every story the catalog renders after it, which is why the panel story
 * has always been the only one allowed to touch it. Restoring the default here
 * is what buys this file the second state.
 */
function KeepTheDefault({ children }: Readonly<{ children: ReactNode }>) {
  useEffect(() => () => setEdgeLightShown(true), []);
  return <>{children}</>;
}

function story(lit: boolean) {
  return () => {
    // Before the render rather than in an effect: the switch reads the store on
    // its first pass, and a flip one frame later is a story that documents the
    // state it was leaving.
    setEdgeLightShown(lit);
    return (
      <StoryProviders>
        <KeepTheDefault>
          <EdgeLightSetting />
        </KeepTheDefault>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof EdgeLightSetting> = {
  title: "Shell/Agent edge light",
  component: EdgeLightSetting,
};
export default meta;
type Story = StoryObj<typeof EdgeLightSetting>;

/** On, which is the default: the lit rim is how the product says the agent is
 *  working, so an installation nobody has told otherwise shows it. */
export const Lit: Story = { render: story(true) };

/** Off. Turning it off costs a reading rather than a fact — everything the
 *  margin carries is also written in words in the rail beside it. */
export const Unlit: Story = { render: story(false) };

/** The same row in dark, where the track is the one filled control on a panel
 *  whose every other surface is a hairline. */
export const LitDark: Story = {
  globals: { theme: "dark" },
  render: story(true),
};

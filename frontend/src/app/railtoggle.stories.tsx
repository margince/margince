// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef, useState } from "react";
import { expect, waitFor, within } from "storybook/test";
import { StoryProviders } from "../testing/story-utils";
import { useNavTip } from "./navtip";
import { NavRailToggle } from "./railtoggle";

// The toggle row at the rail's foot, in the `.app` grid that gives the rail its
// width. A press moves it to the other state.
function RailFoot({ startCollapsed }: Readonly<{ startCollapsed: boolean }>) {
  const [collapsed, setCollapsed] = useState(startCollapsed);
  const nav = useRef<HTMLElement | null>(null);
  const state = useNavTip(collapsed, nav);
  return (
    <StoryProviders>
      <div className={collapsed ? "app" : "app railexpanded"}>
        <nav
          ref={nav}
          className={collapsed ? "rail collapsed" : "rail expanded"}
        >
          <div className="grow" />
          <NavRailToggle
            state={state}
            onToggle={() => setCollapsed((current) => !current)}
          />
        </nav>
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof NavRailToggle> = {
  title: "Shell/Rail toggle",
  component: NavRailToggle,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof NavRailToggle>;

/** Open: the row reads "Collapse sidebar". */
export const Expanded: Story = {
  render: () => <RailFoot startCollapsed={false} />,
};

/** Folded: the glyph alone, named "Expand sidebar", with its tip on focus. */
export const Collapsed: Story = {
  render: () => <RailFoot startCollapsed />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    canvas.getByRole("button", { name: "Expand sidebar" }).focus();
    await waitFor(() =>
      expect(canvas.getByRole("tooltip").textContent).toMatch(
        /^Expand sidebar · (⌘|Ctrl) B$/,
      ),
    );
  },
};

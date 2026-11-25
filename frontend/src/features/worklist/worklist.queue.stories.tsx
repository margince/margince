// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { useWorklistPicks, WorklistSelect } from "./worklist.bulkdone";
import { row } from "./worklist.fixtures";
import { ViewPanel } from "./worklist.queue";

const rows = {
  positions: new Map(),
  owner: "",
  selectedId: "",
  onSelect: fn(),
  onOpenEmail: fn(),
  onView: fn(),
  picks: { offered: false, picked: new Set<string>(), toggle: fn() },
};

// The Decisions and System views draw their rows in one panel named by the view.
const meta: Meta<typeof ViewPanel> = {
  title: "Records/Worklist/View panel",
  component: ViewPanel,
  decorators: [
    (Story, context) => {
      stubWithSession({}, {});
      return (
        <StoryProviders
          locale={
            context.parameters.locale === "de"
              ? "de"
              : context.parameters.locale === "vi"
                ? "vi"
                : "en"
          }
        >
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    view: "decisions",
    items: [
      row({
        source: "dedupe_candidate",
        category: "decisions",
        destination: "review",
      }),
    ],
    rows,
  },
};
export default meta;
type Story = StoryObj<typeof ViewPanel>;
export const Decisions: Story = {
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("Decisions")).toBeVisible();
  },
};
export const DecisionsDark: Story = {
  ...Decisions,
  globals: { theme: "dark" },
};
export const System: Story = {
  args: {
    view: "system",
    items: [row({ source: "capture_health", category: "system" })],
  },
};
export const SystemDark: Story = { ...System, globals: { theme: "dark" } };
export const DecisionsGerman: Story = {
  ...Decisions,
  parameters: { locale: "de" },
  play: undefined,
};
export const DecisionsVietnamese: Story = {
  ...Decisions,
  parameters: { locale: "vi" },
  play: undefined,
};

const pickable = [
  row({
    source: "dedupe_candidate",
    category: "decisions",
    destination: "review",
    dispositions: ["handled"],
  }),
];

// Select stands beside the Company questions launcher in the head, and the
// head wraps the two rather than drawing one over the other.
function DecisionsSelecting() {
  const selection = useWorklistPicks(pickable, "decisions");
  return (
    <div style={{ inlineSize: 320 }}>
      <ViewPanel
        view="decisions"
        items={pickable}
        rows={{ ...rows, picks: selection.picks }}
        select={<WorklistSelect selection={selection} />}
      />
    </div>
  );
}

export const DecisionsWithSelectNarrow: Story = {
  render: () => <DecisionsSelecting />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const launcher = canvas
      .getByRole("button", { name: "Company questions" })
      .getBoundingClientRect();
    const select = canvas
      .getByRole("button", { name: "Select" })
      .getBoundingClientRect();
    const apart =
      select.left >= launcher.right ||
      select.right <= launcher.left ||
      select.top >= launcher.bottom ||
      select.bottom <= launcher.top;
    await expect(apart).toBe(true);
  },
};
export const DecisionsWithSelectNarrowDark: Story = {
  ...DecisionsWithSelectNarrow,
  globals: { theme: "dark" },
};

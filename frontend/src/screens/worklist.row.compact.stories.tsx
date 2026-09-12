// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";
import "./worklist.css";

// The row at LIST DENSITY — one line per piece of work.
//
// Every frame goes through `WorklistRow density="compact"` rather than through
// the line component underneath it, because what is worth looking at is the
// row a surface actually mounts: the kind column, the verbs and the thumb
// surface are the row's at both densities, and a frame that drew the line on
// its own would be a picture of a part nobody ships.
//
// What to check in each frame:
//   · the row is ONE line, and the verbs are on it rather than under it;
//   · the name is the only control reaching the record — no second "Open";
//   · the fragments beside the name clip with an ellipsis and carry the whole
//     string on hover AND on focus;
//   · the count at the end is as quiet as the fragments and louder than
//     nothing, and it opens beside the line rather than pushing it down.
//
// Both themes. The fragments are `--textMeta` over the panel's translucent
// ground and the count is a ghost control on the same ground — both are
// `color-mix()` over canonical tokens and both re-resolve on the flip.

type WorklistItem = components["schemas"]["WorklistItem"];

const DEAL = "01a05500-0000-7000-8000-0000000000da";

/** A deal the night picked out, with more to say than one line can hold. */
function dealRow(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "brief_item",
    category: "deals_at_risk",
    level: 2,
    consequence: "revenue_slips",
    title: "Turbinenbau retrofit",
    because: [
      { kind: "quiet_days", value: { kind: "days", days: 21 } },
      { kind: "closing_soon" },
      {
        kind: "expected_revenue",
        value: { kind: "money", minor: 16_010_000, currency: "EUR" },
      },
    ],
    actions: ["open"],
    dispositions: ["snooze"],
    deal: {
      amount_minor: 16_010_000,
      currency: "EUR",
      expected_close_date: "2026-09-30",
      quiet_days: 21,
    },
    subject: { type: "deal", id: DEAL, label: "Turbinenbau retrofit" },
    ...over,
  } as WorklistItem;
}

/** Nothing this row's frames press reaches a real API. */
function stubRow() {
  globalThis.fetch = (async (): Promise<Response> =>
    jsonResponse({ data: [] })) as typeof fetch;
}

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/Row at list density",
  component: WorklistRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="Today">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistRow>;

const COMPACT = { density: "compact", owner: "" } as const;

// The shape itself: name, then when/worth/why/cost as one dot-separated
// fragment, then the verbs. No rank column — the ordered list is what carries
// the order, and a digit per row spends a column saying it again.
export const OneLine: Story = {
  args: { ...COMPACT, item: dealRow() },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};

// PINNED. The reader's own override arrives as a reason like any other, so the
// line says it among the rest and the pin glyph at the end offers to lift it.
export const Pinned: Story = {
  args: {
    ...COMPACT,
    item: dealRow({
      because: [
        { kind: "pinned" },
        { kind: "quiet_days", value: { kind: "days", days: 21 } },
      ],
    }),
  },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};

// MORE THAN THE LINE HOLDS: six reasons, the comparison with the row below, and
// a supporting sentence. Three are said outright and the count at the end names
// everything else — a count that promised only the reasons would promise less
// than the press delivers.
export const WithMoreBehindIt: Story = {
  args: {
    ...COMPACT,
    item: dealRow({
      detail: "The last four messages were all outbound.",
      because: [
        { kind: "quiet_days", value: { kind: "days", days: 21 } },
        { kind: "closing_soon" },
        {
          kind: "expected_revenue",
          value: { kind: "money", minor: 16_010_000, currency: "EUR" },
        },
        { kind: "no_champion" },
        { kind: "pinned" },
      ],
      above_next: {
        comparator: "deadline",
        mine: { kind: "date", date: "2026-09-30T09:00:00Z" },
        theirs: { kind: "date", date: "2026-11-15T09:00:00Z" },
      },
    }),
  },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};

// The same row with the count pressed. The panel is portalled beside the line,
// so the rows under it have not moved — which is the whole reason this is a
// popover and not the fold the default density draws.
export const MoreOpen: Story = {
  args: WithMoreBehindIt.args,
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // By ROLE and by its own name: the count IS the trigger, so a frame that
    // reached for it by class would pass over a press a reader cannot make.
    await userEvent.click(await canvas.findByRole("button", { name: /more/ }));
  },
};

// A row with nothing beside its name: no clock, no figures, no reasons. The
// fragment and the count are both absent rather than drawn empty — a line of
// furniture on every quiet row is what a reader learns to look past.
export const NothingBesideTheName: Story = {
  args: {
    ...COMPACT,
    item: dealRow({
      because: [],
      // `none` is the wire's own "nothing happens if you leave it", not an
      // omission: the field is required, and a story that dropped it would
      // draw a payload the server cannot send.
      consequence: "none",
      deal: undefined,
      detail: undefined,
    }),
  },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};

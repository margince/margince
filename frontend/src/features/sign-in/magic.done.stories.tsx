// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import type { MagicLine, MagicReceipt } from "../../lib/magic.queries";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { expectTextFits, UNBROKEN_SUBJECT } from "../../testing/unbrokentext";
import { MagicPanel } from "./magic";

// The last card of Done for you on Home: a night of mailbox sync, with
// repeats that group, a filing and an archive of two.

const meta: Meta<typeof MagicPanel> = {
  title: "Shell/Home/Small changes",
  component: MagicPanel,
};
export default meta;
type Story = StoryObj<typeof MagicPanel>;

const SYNC: MagicLine["actor"] = {
  type: "system",
  id: "capture",
  label: { key: "magic.by.mail_filing" },
};
const NO_WAY_BACK = {
  undoable: false,
  reason: "not_a_replayable_verb",
} as const;

function small(
  n: number,
  key: string,
  label: string,
  over: Partial<MagicLine> = {},
): MagicLine {
  return {
    id: `01a00000-0000-7000-8000-0000000005${10 + n}`,
    occurred_at: `2026-09-13T0${9 - n}:${10 + n}:00Z`,
    lane: "done",
    summary: { key },
    actor: SYNC,
    undo: NO_WAY_BACK,
    entity: {
      type: "activity",
      id: `01a00000-0000-7000-8000-0000000006${10 + n}`,
      label,
    },
    ...over,
  };
}

// A night of mailbox sync: repeats that group, a filing, a calendar update.
const SMALL_CHANGES: MagicLine[] = [
  small(
    0,
    "magic.action.archive_activity",
    "Modernizing your data foundation on a budget",
    { count: 2 },
  ),
  small(
    1,
    "magic.action.create_activity",
    "Update to the Intuit Developer Terms of Service",
  ),
  small(
    2,
    "magic.action.create_activity",
    "Modernizing your data foundation on a budget",
    { count: 2 },
  ),
  small(
    3,
    "magic.action.activity_relink",
    "Re: Margince follow-up: recording, what we heard, and Tuesday",
  ),
  small(
    4,
    "magic.action.create_activity",
    "[GitHub] Claude is requesting updated permissions",
  ),
];

const NIGHT: MagicReceipt = {
  as_of: "2026-09-13T09:00:00Z",
  since: "2026-09-12T18:00:00Z",
  done: SMALL_CHANGES,
  needs_you: [],
  could_not_complete: [],
  watching: [],
  totals: {
    done: SMALL_CHANGES.length,
    needs_you: 0,
    could_not_complete: 0,
    watching: 0,
  },
  not_shown: [],
  sources_unavailable: [],
};

// The done lane as Home embeds it under the Done for you band.
function smallChanges(receipt: MagicReceipt = NIGHT) {
  stubWithSession(
    { "GET /magic": () => jsonResponse(receipt) },
    { deal: ["read", "update"] },
  );
  return (
    <StoryProviders>
      <section className="brief-section brief-done">
        <MagicPanel lanes={["done"]} embedded />
      </section>
    </StoryProviders>
  );
}

/** Expanding the count reveals the grouped changes and their controls. */
export const Expanded: Story = {
  render: () => smallChanges(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("7 small changes"));
    await expect(await canvas.findByText("4 changes")).toBeVisible();
  },
};

/** A night with no deal changes still starts closed. */
export const Alone: Story = {
  render: () => smallChanges(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const count = await canvas.findByText("7 small changes");
    await expect(count.closest("summary")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  },
};

/** Why a change has no Undo waits in its Details, not on the line. */
export const DetailsOpened: Story = {
  render: Alone.render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("7 small changes"));
    const [, details] = await canvas.findAllByText("Details");
    if (!details) throw new Error("no Details");
    await userEvent.click(details);
    // Every line holds its own note; the one to read is in the line opened.
    const opened = details.closest("details");
    if (!opened) throw new Error("Details is not a disclosure");
    await expect(
      await within(opened).findByText(
        englishCopy["history.undo.notReplayable"],
      ),
    ).toBeVisible();
  },
};

export const AloneDark: Story = {
  globals: { theme: "dark" },
  render: Alone.render,
};

export const AlonePhone: Story = {
  render: Alone.render,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const UNBROKEN: MagicReceipt = {
  ...NIGHT,
  done: [
    small(0, "magic.action.create_activity", UNBROKEN_SUBJECT, { count: 5 }),
  ],
  totals: { ...NIGHT.totals, done: 1 },
};

/** A folded line's unbroken name wraps inside its row; its opener stays on it. */
export const UnbrokenNamePhone: Story = {
  render: () => smallChanges(UNBROKEN),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("5 small changes"));
    const name = await canvas.findByText(`${UNBROKEN_SUBJECT} and 4 more`);
    const row = name.closest<HTMLElement>(".brief-done-controls");
    if (!row) throw new Error("the folded name is not on a change's row");
    await expectTextFits(name, row.getBoundingClientRect());
    await expect(within(row).getByText("View 5 changes")).toBeVisible();
  },
};

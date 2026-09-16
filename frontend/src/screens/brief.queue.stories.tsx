// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { announceAddressChanged } from "../app/router";
import { en } from "../i18n/en";
import {
  leadRow,
  meetingRow,
  readingsDay,
  taskRow,
  type Worklist,
  waitingEmailRow,
} from "./brief.fixtures";
import { BriefQueue } from "./brief.queue";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

function QueueFrame({ day }: Readonly<{ day?: Worklist }>) {
  useEffect(() => {
    const before = window.location.hash;
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /worklist": () =>
        jsonResponse(
          day ??
            readingsDay(
              {},
              [
                {
                  ...taskRow("buyer", "Send the promised rollout comparison"),
                  contact: {
                    id: "contact-sonya",
                    label: "Sonya Beck",
                    touch: {
                      last_inbound_at: "2026-09-03T16:46:00Z",
                      last_outbound_at: "2026-08-28T09:12:00Z",
                    },
                  },
                },
              ],
              [
                {
                  category: "tasks",
                  considered: 1,
                  shown: 1,
                  more_available: false,
                },
              ],
              { in_play: 1 },
            ),
        ),
      "GET /worklist/handled": () =>
        jsonResponse({
          as_of: "2026-09-13T08:00:00Z",
          receipts: [],
          truncated: false,
        }),
    });
    window.location.hash = "#/home?queue=1";
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
    // The day this frame serves. A story hands it once and never changes it,
    // but the stub is installed inside the effect, so a frame that swapped its
    // fixture would otherwise keep serving the first one.
  }, [day]);
  return (
    <StoryProviders>
      <BriefQueue />
    </StoryProviders>
  );
}

const meta: Meta<typeof BriefQueue> = {
  title: "Shell/Home work queue",
  component: BriefQueue,
  render: () => <QueueFrame />,
};
export default meta;
type Story = StoryObj<typeof BriefQueue>;
export const Desktop: Story = {};
export const Phone: Story = { tags: ["uat-phone"] };

/**
 * A FULL DAY read by a lead, which is the state every part of the head has
 * something to do in: both dials are offered, the readings carry figures, and
 * the queue is long enough to page.
 *
 * What to check, because each was a defect in the drawer and in no other
 * frame:
 *   · the day's figures, the scope switch and the Viewing picker stand on ONE
 *     line — the picker's own label used to push it under the switch and leave
 *     a band of empty ground beside the figures;
 *   · the four readings stay ONE row. They are four, and the strip's fold
 *     ladder was written for a row of six, so it folded them to three and let
 *     the fourth span the rest;
 *   · pressing a row opens NO column beside the queue. The row already names
 *     the contact and both moments, and a drawer has no third of its width to
 *     spend saying it again.
 */
export const AFullDay: Story = {
  render: () => <QueueFrame day={aLeadsDay()} />,
  // The DOCUMENT, not the canvas: this drawer is a `Modal` and portals out of
  // the story's own root, so a canvas-scoped query finds an empty div and the
  // play reports a missing control rather than a missing portal.
  play: async () => {
    const drawer = within(document.body);
    // The readings open folded in the drawer — the work comes first — so the
    // frame opens them to show the row this story is about. The trigger is the
    // disclosure's own `<summary>`, which carries no button role to query by.
    // WAITED FOR, not read once: the drawer fetches its day before it draws
    // anything, so a synchronous query runs against an empty dialog and reports
    // a missing control rather than one that has not arrived.
    const summary = await waitFor(() => {
      const found = [...document.querySelectorAll(".disclosure-summary")].find(
        (node) => node.textContent?.includes(en["brief.readings.summary"]),
      );
      if (!found) {
        throw new Error("the drawer has not drawn its readings disclosure");
      }
      return found as HTMLElement;
    });
    await userEvent.click(summary);
    await userEvent.click(
      (await drawer.findAllByRole("button", { name: /Meet next Tues/ }))[0],
    );
    // The row is in hand and NO column opened beside it — the row already
    // names the contact and both moments, and the drawer has no width to
    // spend saying it twice.
    await expect(
      drawer.queryByRole("complementary", { name: en["worklist.pane.title"] }),
    ).toBeNull();
  },
};

/** The same day in dark, where the head's band and the readings re-derive. */
export const AFullDayInDark: Story = {
  ...AFullDay,
  globals: { theme: "dark" },
};

/** A lead's day: every scope on offer, four readings with figures, and enough
 *  rows that the queue is a list rather than a single entry. */
function aLeadsDay(): Worklist {
  const queue = [
    waitingEmailRow(),
    meetingRow("m1", false),
    leadRow("l1"),
    taskRow("t1", "Send the promised rollout comparison"),
    taskRow("t2", "Prepare the revised comparison sheet"),
  ];
  return {
    ...readingsDay(
      {
        revenue_at_risk_minor: null,
        buyer_replies: 44,
        prospecting: 1,
        review: 2,
      },
      queue,
      [
        {
          category: "customer_waiting",
          considered: 44,
          shown: 1,
          more_available: true,
        },
        {
          category: "meetings",
          considered: 9,
          shown: 1,
          more_available: false,
        },
        { category: "tasks", considered: 1, shown: 2, more_available: false },
        {
          category: "decisions",
          considered: 2,
          shown: 0,
          more_available: false,
        },
      ],
      { urgent: 3, due: 0, in_play: 11, lower_priority: 55, total: 69 },
    ),
    scope_options: ["mine", "unassigned", "team", "all"],
  };
}

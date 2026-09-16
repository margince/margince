// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { expect, userEvent, within } from "storybook/test";
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
 * something to do in: both dials are offered and the queue is long enough to
 * page.
 *
 * What to check, because each was a defect in the drawer and in no other
 * frame:
 *   · the day's figures, the scope switch and the Viewing picker stand on ONE
 *     line — the picker's own label used to push it under the switch and leave
 *     a band of empty ground beside the figures;
 *   · the head is followed by the QUEUE and nothing else. The readings the
 *     page behind this drawer already carries were drawn here too, folded
 *     behind a disclosure — the same figures twice on one screen, with a rule
 *     and a word of chrome between the head and the work;
 *   · a rule falls between EVERY pair of rows, the seam between the banded
 *     run and the unbanded tail included. They are separate lists, so the
 *     tail's first row used to keep the reset a panel's first row gets;
 *   · every row carries its verbs, in the card's order;
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

/** The same day in dark, where the head's band and the rows re-derive. */
export const AFullDayInDark: Story = {
  ...AFullDay,
  globals: { theme: "dark" },
};

/** A lead's day: every scope on offer and enough rows that the queue is a list
 *  rather than a single entry.
 *
 * Every row carries the SUBJECT, the set-asides and the prepared move the
 * server really sends, because those are what a row's verbs are drawn from: a
 * fixture with `actions: ["open"]` and no subject routes that verb nowhere, so
 * the row comes out with the pin and nothing else — a frame of the queue with
 * no way to work it, which is not a state the product has. */
function aLeadsDay(): Worklist {
  const meeting = {
    ...meetingRow("m1", false),
    subject: {
      type: "contact" as const,
      id: "contact-weber",
      label: "Nils Weber",
    },
    // The brief opens on the CONTACT's record and names the meeting, so the
    // move needs both ids — `briefHref` draws nothing without them.
    with_contact: "contact-weber",
    move: { action: "open_meeting_brief" as const, activity_id: "m1" },
    dispositions: ["snooze" as const],
  };
  const lead = {
    ...leadRow("l1"),
    subject: {
      type: "lead" as const,
      id: "lead-weber",
      label: "Weber GmbH",
    },
    move: { action: "draft_email" as const },
    dispositions: ["snooze" as const, "not_sales" as const],
  };
  const queue = [
    waitingEmailRow(),
    meeting,
    lead,
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

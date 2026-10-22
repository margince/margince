// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { linkedInReachFixture } from "../../testing/reachfixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectionOutcomesCard } from "./connection-outcomes";

// One row per list: a count, and a verb that opens the list only when there
// is something in it. The drawers have stories beside the lists they draw.

type Lists = Readonly<{
  senders: number;
  waiting: number;
  held: number;
  reach: number;
}>;

function ids<T>(count: number, make: (i: number) => T): T[] {
  return Array.from({ length: count }, (_, i) => make(i));
}

type Read = "ok" | "failed" | "pending";

const failedRead = () => jsonResponse({ title: "Internal Server Error" }, 500);

function story(
  lists: Lists,
  reads: Readonly<{ held?: Read; waiting?: Read }> = {},
) {
  const answer = (read: Read | undefined, ok: () => Response) => () => {
    if (read === "failed") return failedRead();
    if (read === "pending") return new Promise<Response>(() => {});
    return ok();
  };
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/senders": () =>
        jsonResponse({
          data: ids(lists.senders, (i) => ({
            address: `sender${i}@example.com`,
            kind: "contact",
            overruled: false,
            record_exists: false,
            message_count: 1,
          })),
        }),
      "GET /capture/contacts-awaiting-decision": answer(reads.waiting, () =>
        jsonResponse({
          data: ids(lists.waiting, (i) => ({
            contact_id: `0190b7a2-0000-7000-8000-00000000003${i}`,
            emails: [`waiting${i}@example.com`],
            captured_at: "2026-09-10T14:40:00Z",
          })),
        }),
      ),
      "GET /capture/held-threads": answer(reads.held, () =>
        jsonResponse({
          data: ids(lists.held, (i) => ({
            thread_key: `t-${i}`,
            status: "held",
            pending: false,
            attempts: 1,
            has_message: true,
            kind: "legal",
          })),
        }),
      ),
      "GET /me/linkedin-reach": () =>
        jsonResponse(
          linkedInReachFixture(
            ids(lists.reach, (i) => ({
              company_id: `018f3a1b-0000-7000-8000-0000000000b${i}`,
              display_name: `Company ${i + 1}`,
              connections: 3,
              contacts_on_file: 1,
            })),
          ),
        ),
    });
    return (
      <StoryProviders>
        <ConnectionOutcomesCard />
      </StoryProviders>
    );
  };
}

const NOTHING: Lists = { senders: 0, waiting: 0, held: 0, reach: 0 };
const SOME: Lists = { senders: 24, waiting: 2, held: 3, reach: 1 };

const meta = {
  title: "Settings/You/Capture activity/Connection outcomes",
  component: ConnectionOutcomesCard,
} satisfies Meta<typeof ConnectionOutcomesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

// A fresh seat: every row reads "None yet" and offers nothing to open.
export const NothingYet: Story = {
  render: story(NOTHING),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findAllByText("None yet")).toHaveLength(3);
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

export const SomeOfEach: Story = {
  render: story(SOME),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("24 senders")).toBeVisible();
    await expect(
      canvas.getAllByRole("button", { name: /^View / }),
    ).toHaveLength(4);
  },
};

// A list that failed to load says so on its row rather than reading as empty.
export const OneListFailed: Story = {
  render: story(SOME, { held: "failed" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Could not load")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Retry Held threads" }),
    ).toBeVisible();
  },
};

// A queue that could not be read may hold somebody, so its row stays.
export const WaitingFailed: Story = {
  render: story(NOTHING, { waiting: "failed" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", {
        name: "Retry Waiting on a decision",
      }),
    ).toBeVisible();
  },
};

// A list still loading holds its place with a bar, never "None yet".
export const OneListLoading: Story = {
  render: story(SOME, { held: "pending" }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("24 senders")).toBeVisible();
    await expect(canvasElement.querySelectorAll(".skeleton")).toHaveLength(1);
  },
};

export const SomeOfEachDark: Story = {
  globals: { theme: "dark" },
  render: story(SOME),
};

export const SomeOfEachPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(SOME),
};

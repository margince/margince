// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import { ListHistoryPanel } from "./listhistory";
import {
  history,
  listsMe,
  liveHistory,
  liveList,
  MEMBER_ID,
  shortlist,
} from "./lists.fixtures";
import type { List, ListHistoryEntry } from "./lists.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// What changed on a list, newest first. A Live List says above its rows when
// its checks last ran, with how they work one press away; a Shortlist's rows
// say who chose each record and, under it, why.
const meta: Meta = {
  title: "Records/List history",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** A record an automation rule put on the Shortlist, run as its owner. */
const addedByRule: ListHistoryEntry = {
  id: "01a0f000-0000-7000-8000-000000000014",
  kind: "member_added",
  occurred_at: "2026-09-22T07:00:00Z",
  actor: "human:00000000-0000-4000-8000-000000000001",
  actor_name: "Lena Vogt",
  entity_type: "company",
  entity_id: MEMBER_ID,
  reason: "automation",
};

function panel(list: List, rows: readonly ListHistoryEntry[]) {
  return () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${list.id}/history`]: () =>
        jsonResponse({ data: rows, page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <ListHistoryPanel list={list} />
      </StoryProviders>
    );
  };
}

// The checks saw one record join after the filter changed and one leave.
export const LiveHistory: Story = { render: panel(liveList, liveHistory) };

// No check has reached the list yet, so nobody has joined or left.
export const NotChecked: Story = {
  render: panel({ ...liveList, last_check: undefined }, []),
};

// The last check matched too many records to say who joined and left.
export const TooLarge: Story = {
  render: panel(
    {
      ...liveList,
      last_check: { checked_at: "2026-09-29T08:15:00Z", outcome: "too_large" },
    },
    [],
  ),
};

// The note a reader left on a chosen record sits on its own line under the
// change, and a record a rule added says so in plain ink.
export const ShortlistWithNote: Story = {
  render: panel(shortlist, [addedByRule, ...history]),
};

export const Empty: Story = { render: panel(shortlist, []) };

// "How checks work", at the end of the intro, pressed: the caveat the sentence
// leaves out, opening from inside the panel rather than from its head.
export const HowChecksWork: Story = {
  ...LiveHistory,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["lists.history.howChecks"],
      }),
    );
  },
};

export const LiveHistoryDark: Story = {
  ...LiveHistory,
  globals: { theme: "dark" },
};

export const ShortlistWithNoteDark: Story = {
  ...ShortlistWithNote,
  globals: { theme: "dark" },
};

// The Popover's panel has to stand off the Panel ground in dark too.
export const HowChecksWorkDark: Story = {
  ...HowChecksWork,
  globals: { theme: "dark" },
};

// At phone width the table scrolls inside its own frame and the page does not.
// `uat-phone` drives the capture gate to 390px.
export const LiveHistoryPhone: Story = {
  ...LiveHistory,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

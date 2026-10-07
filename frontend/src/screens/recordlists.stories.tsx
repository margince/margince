// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { ListsSection } from "./companyraillists";
import {
  LIVE_ID,
  listsMe,
  liveList,
  MEMBER_ID,
  notOnLiveWhy,
  shortlist,
} from "./lists.fixtures";
import { RecordListsPanel } from "./recordlists";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The lists one record is on, on its own page, and the check that says why it
// is not on a Live List.
const meta: Meta = {
  title: "Records/Record lists",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const vocabulary = { resource: "company", fields: [] };

function stub(onLists: readonly unknown[]) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /records/company/${MEMBER_ID}/lists`]: () =>
      jsonResponse({ data: onLists }),
    "GET /lists": () =>
      jsonResponse({ data: [liveList], page: { has_more: false } }),
    [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
      jsonResponse(notOnLiveWhy),
    "GET /filters/vocabulary": () => jsonResponse(vocabulary),
  });
}

// On a Shortlist the reader may change and on a Live List.
export const OnTwoLists: Story = {
  render: () => {
    stub([{ ...shortlist, health: "ok" }, liveList]);
    return (
      <StoryProviders>
        <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
      </StoryProviders>
    );
  },
};

// On no list the reader can find.
export const OnNoList: Story = {
  render: () => {
    stub([]);
    return (
      <StoryProviders>
        <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
      </StoryProviders>
    );
  },
};

// Checking a Live List the record is not on: the clause it fails, with the
// record's value beside what the clause needs, and a hidden value named so.
export const CheckedAListItIsNotOn: Story = {
  render: () => {
    stub([]);
    return (
      <StoryProviders>
        <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("combobox"));
    await userEvent.click(
      await within(document.body).findByRole("option", { name: liveList.name }),
    );
    await expect(await canvas.findByText("Now: Logistics")).toBeVisible();
  },
};

// The same block as a slice of the company rail.
export const InTheCompanyRail: Story = {
  render: () => {
    stub([{ ...shortlist, health: "ok" }, liveList]);
    return (
      <StoryProviders>
        <Panel>
          <ListsSection companyId={MEMBER_ID} />
        </Panel>
      </StoryProviders>
    );
  },
};

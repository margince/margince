// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { ListsSection } from "./companyraillists";
import {
  LIVE_ID,
  listsMe,
  liveList,
  liveVocabulary,
  MEMBER_ID,
  notOnLiveWhy,
  SHORTLIST_ID,
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

function stub(onLists: readonly unknown[]) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /records/company/${MEMBER_ID}/lists`]: () =>
      jsonResponse({ data: onLists }),
    "GET /lists": () =>
      jsonResponse({ data: [liveList], page: { has_more: false } }),
    [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
      jsonResponse(notOnLiveWhy),
    "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
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

const REMOVE = `POST /lists/${SHORTLIST_ID}/members/remove`;
const RESTORE = `POST /lists/${SHORTLIST_ID}/members/restore`;

function TakeOffBench({ restore }: Readonly<{ restore?: () => Response }>) {
  let onList = true;
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /records/company/${MEMBER_ID}/lists`]: () =>
      jsonResponse({ data: onList ? [{ ...shortlist, health: "ok" }] : [] }),
    "GET /lists": () =>
      jsonResponse({ data: [liveList], page: { has_more: false } }),
    "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
    [REMOVE]: () => {
      onList = false;
      return jsonResponse({ audit_id: "0199a000-0000-7000-8000-0000000000b2" });
    },
    [RESTORE]: () => {
      if (restore) {
        return restore();
      }
      onList = true;
      return jsonResponse({});
    },
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

const pressTakeOff: Story["play"] = async ({ canvasElement }) => {
  const user = userEvent.setup();
  await user.click(
    await within(canvasElement).findByRole("button", {
      name: "Take off the Shortlist",
    }),
  );
};

async function undoInToast() {
  const undo = await within(document.body).findByRole("button", {
    name: "Undo",
  });
  await waitFor(() => expect(undo).toBeVisible());
  return undo;
}

/** Taking the record off runs at once: no dialog, no note, and an Undo. */
export const TakenOffWithUndo: Story = {
  render: () => <TakeOffBench />,
  play: async (context) => {
    await pressTakeOff(context);
    await undoInToast();
    await expect(
      within(document.body).queryByRole("dialog"),
    ).not.toBeInTheDocument();
  },
};

/** An Undo the server refuses stays on screen as a danger toast. */
export const UndoRefused: Story = {
  render: () => (
    <TakeOffBench
      restore={() =>
        jsonResponse(
          { detail: "The record was added to this Shortlist again since." },
          409,
        )
      }
    />
  ),
  play: async (context) => {
    await pressTakeOff(context);
    const user = userEvent.setup();
    await user.click(await undoInToast());
    const close = await within(document.body).findByRole("button", {
      name: "Close",
    });
    await waitFor(() => expect(close).toBeVisible());
    await waitFor(() =>
      expect(
        within(document.body).getByText(
          "The record was added to this Shortlist again since.",
        ),
      ).toBeVisible(),
    );
  },
};

export const TakenOffOnAPhone: Story = {
  ...TakenOffWithUndo,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import {
  listsMe,
  MEMBER_ID,
  SHORTLIST_ID,
  shortlist,
} from "../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../testing/story-utils";
import { AddToShortlistAction } from "./addtoshortlist";

// The record page's "Add to Shortlist": the verb, and behind it the choice of
// Shortlist and the note on why. Absent while lists are switched off.
const meta: Meta = { title: "Patterns/Add to Shortlist" };
export default meta;

type Story = StoryObj;

function offered(reasonId?: string, routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": listsMe(true),
    "GET /lists": () =>
      jsonResponse({ data: [shortlist], page: { has_more: false } }),
    ...routes,
  });
  return (
    <StoryProviders>
      <AddToShortlistAction
        entityType="company"
        entityId={MEMBER_ID}
        reasonId={reasonId}
      />
      {reasonId && (
        <p id={reasonId} className="t-caption">
          {en["record.archivedReadOnly"]}
        </p>
      )}
    </StoryProviders>
  );
}

export const Offered: Story = { render: () => offered() };

/** Refused on a record that takes no changes, pointing at the page's reason. */
export const Refused: Story = { render: () => offered("record-refused") };

/** The verb pressed: which Shortlist, and the note on why. */
export const Choosing: Story = {
  render: () => offered(),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Add to Shortlist",
      }),
    );
    await within(document.body).findByRole("dialog");
  },
};

/** The record is already on the chosen Shortlist: said once, on that field. */
export const AlreadyOnIt: Story = {
  render: () =>
    offered(undefined, {
      [`POST /lists/${SHORTLIST_ID}/members`]: () =>
        jsonResponse(
          {
            title: "Conflict",
            status: 409,
            code: "conflict",
            detail: "the record is already on this list: conflict",
          },
          409,
        ),
    }),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Add to Shortlist",
      }),
    );
    const page = within(document.body);
    await userEvent.click(
      await page.findByRole("combobox", { name: "Shortlist" }),
    );
    await userEvent.click(
      await page.findByRole("option", { name: shortlist.name }),
    );
    await userEvent.click(page.getByRole("button", { name: "Add" }));
    await expect(
      await page.findByText("Already on this Shortlist."),
    ).toBeVisible();
  },
};

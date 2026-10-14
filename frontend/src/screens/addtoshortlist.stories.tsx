// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { AddToShortlistAction } from "./addtoshortlist";
import { listsMe, MEMBER_ID, shortlist } from "./lists.fixtures";

// The record page's "Add to Shortlist": the verb, and behind it the choice of
// Shortlist and the note on why. Absent while lists are switched off.
const meta: Meta = { title: "Patterns/Add to Shortlist" };
export default meta;

type Story = StoryObj;

function offered(reasonId?: string) {
  installFetchStub({
    "GET /me": listsMe(true),
    "GET /lists": () =>
      jsonResponse({ data: [shortlist], page: { has_more: false } }),
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

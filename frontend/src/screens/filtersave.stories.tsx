// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { SaveFilterModal } from "./filtersave";
import { listsMe, TEAM_ID, teamsPage } from "./lists.fixtures";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// "Save this filter": one dialog, two answers. A saved view is the reader's
// own; a Live List is shared, so choosing it asks who can find it before
// anything is saved. With lists switched off the dialog asks for a name only.
const meta: Meta = {
  title: "Patterns/Filters and views/Save this filter",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const TREE = newGroup("and", [newLeaf("city", "eq", "Berlin")]);

function routes(
  listsOn: boolean,
  extra: Parameters<typeof installFetchStub>[0] = {},
) {
  installFetchStub({
    "GET /me": listsMe(listsOn, [TEAM_ID]),
    "GET /teams": () => jsonResponse(teamsPage),
    ...extra,
  });
}

function SaveDialog({ keep }: Readonly<{ keep?: "view" | "list" }>) {
  return (
    <StoryProviders>
      <SaveFilterModal
        open
        onClose={() => undefined}
        tab="contacts"
        tree={TREE}
        initialKeep={keep}
        onSaved={() => undefined}
      />
    </StoryProviders>
  );
}

export const ListsOffNameOnly: Story = {
  render: () => {
    routes(false);
    return <SaveDialog />;
  },
};

export const SavedViewChosen: Story = {
  render: () => {
    routes(true);
    return <SaveDialog />;
  },
};

// Who can find it and what it is for appear once the answer is a list that
// other people work from.
export const LiveListChosen: Story = {
  render: () => {
    routes(true);
    return <SaveDialog keep="list" />;
  },
};

// The server's reason, inside the dialog that asked, with the name kept.
export const Refused: Story = {
  render: () => {
    routes(false, {
      "POST /views": () =>
        jsonResponse(
          {
            title: "Unprocessable",
            status: 422,
            detail: "A view with this name already exists.",
          },
          422,
        ),
    });
    return <SaveDialog />;
  },
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.type(
      await page.findByRole("textbox", { name: "Name" }),
      "Berliners",
    );
    await userEvent.click(page.getByRole("button", { name: "Save view" }));
    await page.findByRole("alert");
  },
};

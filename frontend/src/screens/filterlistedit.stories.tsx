// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ListFilterPage } from "./filterlistedit";
import { LIVE_ID, listsMe, liveList, liveVocabulary } from "./lists.fixtures";
import type { List } from "./lists.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// `#/filters/list/<id>`: a Live List's filter, opened straight into its rows
// under the list's own name, and saved back to the list with "Save to {name}".
const meta: Meta<typeof ListFilterPage> = {
  title: "Patterns/Filters and views/List filter",
  component: ListFilterPage,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ListFilterPage>;

const ROWS = Array.from({ length: 25 }, (_, index) => ({
  id: `c-${index}`,
  name: `Maschinenbau ${index + 1} GmbH`,
  industry: "Manufacturing",
}));

function routes(list: List, listsOn = true) {
  installFetchStub({
    "GET /me": listsMe(listsOn),
    [`GET /lists/${LIVE_ID}`]: () => jsonResponse(list),
    "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
    "POST /filters/preview": () =>
      jsonResponse({
        resource: "company",
        match_count: 42,
        columns: ["id", "name", "industry"],
        rows: ROWS,
        truncated: false,
      }),
  });
}

function listFilter(list: List = liveList, listsOn = true) {
  return () => {
    routes(list, listsOn);
    return <ListFilterPage listId={LIVE_ID} />;
  };
}

// Opened from the list page: the list's name heads it, the notice names what
// a save changes, and "Save to" waits for a change.
export const EditingAListFilter: Story = { render: listFilter() };

// Changed: "Save to {name}" is the page's one emerald verb, and Discard puts
// the list's own filter back.
export const Changed: Story = {
  render: listFilter(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const industry = await canvas.findByDisplayValue("Manufacturing");
    await userEvent.clear(industry);
    await userEvent.type(industry, "Retail");
    await canvas.findByText("Unsaved changes");
  },
};

// A list over projects has no builder here: one heading, one sentence, and
// the way back.
export const CannotOpenHere: Story = {
  render: listFilter({ ...liveList, entity_type: "project" }),
};

// Lists switched off for this installation: no list is read at all.
export const ListsOff: Story = { render: listFilter(liveList, false) };

// A list the reader may read but not change: its filter is theirs to start a
// view or a list of their own from, kept the way a new filter is.
export const ReadOnlyList: Story = {
  render: listFilter({ ...liveList, can_edit: false }),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { useT } from "../i18n";
import { StoryProviders } from "../testing/story-utils";
import { type ListPage, ListTable, useListQuery } from "./listquery";
import { mineEmptyNote } from "./recordlist";

// A "Mine" list for a reader who owns nothing. Its "Show all" button goes away
// when pressed, so focus must land on the list's title.

type Row = { id: string; name: string };

const VIEWER = "u-1";

async function noRows(): Promise<ListPage<Row>> {
  return { data: [], page: { next_cursor: null, has_more: false } };
}

function MineList() {
  const t = useT();
  const state = useListQuery<Row>({
    key: "recordlist-story",
    initialFilters: { owner_id: VIEWER },
    fetchPage: noRows,
  });
  return (
    <ListTable
      title="Leads"
      state={state}
      unit="unit.leads"
      columns={[
        {
          key: "name",
          header: t("contacts.name"),
          cell: (row: Row) => row.name,
        },
      ]}
      rowKey={(row) => row.id}
      emptyNote={mineEmptyNote({
        t,
        state,
        viewerId: VIEWER,
        unit: "unit.leads",
      })}
    />
  );
}

const meta = {
  title: "Patterns/Record list/Mine, empty",
  component: MineList,
  parameters: { layout: "padded" },
  beforeEach: () => {
    globalThis.history.replaceState(null, "", "#/leads");
    return () => globalThis.history.replaceState(null, "", "#/");
  },
} satisfies Meta<typeof MineList>;
export default meta;

type Story = StoryObj<typeof meta>;

export const NothingOwned: Story = {
  render: () => (
    <StoryProviders>
      <MineList />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Show all" }),
    );
    await expect(canvas.getByRole("heading", { name: "Leads" })).toHaveFocus();
  },
};

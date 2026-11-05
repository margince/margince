// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "../design-system/pipelineboard";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { type ListPage, type ListQuery, useListQuery } from "./listquery";
import { ListTable } from "./listquerytable";

// The list hands the board drawn as its body the search its rows answer, so a
// match the board cannot show is named above it.
const meta: Meta = {
  title: "Patterns/List query table",
  parameters: { layout: "padded" },
  beforeEach: () => {
    installFetchStub({ "GET /me": meRoute({}) });
    globalThis.history.replaceState(null, "", "#/");
    return () => globalThis.history.replaceState(null, "", "#/");
  },
};
export default meta;

type Story = StoryObj;

type Card = BoardRecord & { stage: string };

const STAGES = [
  { stage: "open", label: "Discovery" },
  { stage: "qualified", label: "Qualified" },
  { stage: "proposal", label: "Proposal" },
  { stage: "won", label: "Closed Won" },
];

const CARDS: Card[] = [
  { id: "d-1", name: "Harbour lighting", stage: "open" },
  { id: "d-2", name: "Depot rollout", stage: "qualified" },
  { id: "d-3", name: "Fleet retrofit", stage: "proposal" },
  { id: "d-4", name: "Fraunhofer tender", stage: "won" },
  { id: "d-5", name: "Fraunhofer renewal", stage: "won" },
];

async function searchCards(query: ListQuery): Promise<ListPage<Card>> {
  const word = query.q.toLowerCase();
  return {
    data: CARDS.filter((card) => card.name.toLowerCase().includes(word)),
    page: { next_cursor: null, has_more: false },
  };
}

function columnsOf(rows: readonly Card[]): BoardColumn<Card>[] {
  return STAGES.map(({ stage, label }) => ({
    stage,
    label,
    deals: rows.filter((card) => card.stage === stage),
  }));
}

function BoardList() {
  const state = useListQuery<Card>({
    key: "story-board",
    initialSort: "-created_at",
    fetchPage: searchCards,
  });
  return (
    // Narrow enough that the last stage starts off screen.
    <div style={{ inlineSize: 640 }}>
      <ListTable
        state={state}
        unit="unit.deals"
        columns={[{ key: "name", header: "Name", cell: (row) => row.name }]}
        rowKey={(row) => row.id}
        body={
          <PipelineBoard
            variant="plain"
            columns={columnsOf(state.rows)}
            renderCard={(card) => <span>{card.name}</span>}
          />
        }
      />
    </div>
  );
}

// A search whose only matches sit in the last stage. Once the rows answer it,
// the board says where they are and scrolls that stage in.
export const SearchAnsweredOffScreen: Story = {
  render: () => (
    <StoryProviders>
      <BoardList />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", { name: "Search" }),
      "fraunhofer",
    );
    await expect(
      await canvas.findByRole("button", { name: "Go to column" }),
    ).toBeTruthy();
    await expect(canvas.getByText("2 results in Closed Won")).toBeTruthy();
    const board = canvasElement.querySelector(".board");
    await waitFor(() => expect(board?.scrollLeft ?? 0).toBeGreaterThan(0));
  },
};

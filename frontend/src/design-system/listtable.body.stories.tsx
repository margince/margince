// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { ListTable } from "./listtable";

// A generic component takes no `component` here: Storybook would have to infer
// `Row` from nothing to derive the args table.
const meta: Meta = {
  title: "Components/Text and data display/List table body",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Deal = { id: string; name: string; stage: string };

const STAGES = ["Qualified", "Proposal", "Negotiation"];
const DEALS: readonly Deal[] = [
  { id: "d1", name: "Harbour cranes", stage: "Qualified" },
  { id: "d2", name: "Fleet renewal", stage: "Proposal" },
  { id: "d3", name: "Cold store", stage: "Negotiation" },
];

// A stand-in for the pipeline board. It draws every stage, even one with no
// deal in it, so it cannot say on its own that nothing matched.
function StageColumns({ deals }: Readonly<{ deals: readonly Deal[] }>) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(3, 1fr)",
        gap: "var(--space-3)",
      }}
    >
      {STAGES.map((stage) => (
        <section key={stage} aria-label={stage}>
          <strong>{stage}</strong>
          <ul>
            {deals
              .filter((deal) => deal.stage === stage)
              .map((deal) => (
                <li key={deal.id}>{deal.name}</li>
              ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

// A search that matches nothing, over a body of its own. The caller says the
// body settled empty, so the grid's no-match line and its Clear head it.
export const SettledEmptyBody: Story = {
  render: () => {
    function Holder() {
      const [search, setSearch] = useState("zzz");
      const needle = search.trim().toLowerCase();
      const shown = DEALS.filter((deal) =>
        deal.name.toLowerCase().includes(needle),
      );
      return (
        <ListTable<Deal>
          rows={shown}
          columns={[
            {
              key: "name",
              header: "Deal",
              fixed: true,
              cell: (deal) => deal.name,
            },
          ]}
          rowKey={(deal) => deal.id}
          unit="deals"
          search={{ value: search, answered: search, onChange: setSearch }}
          body={<StageColumns deals={shown} />}
          bodyOwnsPaging
          bodyEmpty={shown.length === 0}
        />
      );
    }
    return <Holder />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(/No deals match these filters\./);
    expect(canvas.getByRole("region", { name: "Proposal" })).toBeTruthy();
    await userEvent.click(
      canvas.getByRole("button", { name: "Clear filters" }),
    );
    await canvas.findByText("Fleet renewal");
    expect(canvas.queryByText(/No deals match these filters\./)).toBeNull();
  },
};

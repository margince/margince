import type { Meta, StoryObj } from "@storybook/react-vite";
import { CellStack } from "./cellstack";
import { DataTable } from "./datatable";

const meta: Meta<typeof CellStack> = {
  title: "Design System/Cell stack",
  component: CellStack,
};
export default meta;

type Story = StoryObj<typeof CellStack>;

type Lane = { tier: string; at: string; sentinel: string };

const LANES: Lane[] = [
  { tier: "cheap_cloud", at: "04/09/2026, 15:29", sentinel: "provider_quota" },
  { tier: "embed", at: "04/09/2026, 15:34", sentinel: "provider_error" },
];

const TIER = {
  key: "tier",
  header: "Tier",
  render: (row: Lane) => row.tier,
};

// Both shapes, because the defect is only legible as a comparison: the second
// table is what the model-lane page actually drew before this component, and
// the two spans in its cell are the same two spans the first table stacks.
export const StackedAndNot: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <DataTable
        label={"Model lanes"}
        columns={[
          TIER,
          {
            key: "last",
            header: "Last answer",
            render: (row: Lane) => (
              <CellStack>
                <span>{row.at}</span>
                <span className="t-caption">{row.sentinel}</span>
              </CellStack>
            ),
          },
        ]}
        rows={LANES}
        rowKey={(row) => row.tier}
      />
      <DataTable
        label={"Model lanes, unstacked"}
        columns={[
          TIER,
          {
            key: "last",
            header: "Last answer",
            render: (row: Lane) => (
              <span>
                <span>{row.at}</span>
                <span className="t-caption">{row.sentinel}</span>
              </span>
            ),
          },
        ]}
        rows={LANES}
        rowKey={(row) => row.tier}
      />
    </div>
  ),
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { DataTable } from "./datatable";
import { KeyedName } from "./keyedname";

const meta = {
  title: "Components/Text and data display/Keyed name",
  component: KeyedName,
  args: { name: "Everyday cloud", code: "cheap_cloud" },
} satisfies Meta<typeof KeyedName>;
export default meta;

type Story = StoryObj<typeof meta>;

export const NameOverKey: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Everyday cloud");
    await expect(canvas.getByText("cheap_cloud").tagName).toBe("CODE");
  },
};

export const KeyWithoutName: Story = {
  args: { name: "nightly_batch", code: "nightly_batch" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findAllByText("nightly_batch")).toHaveLength(1);
  },
};

type Tier = { name: string; key: string; model: string };

const TIERS: Tier[] = [
  { name: "Small local", key: "local_small", model: "gemma3" },
  { name: "Everyday cloud", key: "cheap_cloud", model: "gemini-3-flash" },
];

export const InATable: Story = {
  render: () => (
    <DataTable
      label="Model tiers"
      rows={TIERS}
      rowKey={(row) => row.key}
      columns={[
        {
          key: "tier",
          header: "Tier",
          render: (row) => <KeyedName name={row.name} code={row.key} />,
        },
        { key: "model", header: "Model", render: (row) => row.model },
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await expect(canvas.getByText("local_small")).toBeTruthy();
  },
};

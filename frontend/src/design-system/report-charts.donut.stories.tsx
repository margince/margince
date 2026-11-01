import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { DonutChart } from "./report-charts.donut";

const meta: Meta = {
  title: "Components/Text and data display/Donut chart",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 620 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

// Parts of one whole in an order: steps of one hue, deepest first.
export const Donut: Story = {
  render: () => (
    <DonutChart
      label="Open pipeline by stage"
      total="€1.3m"
      totalLabel="Total"
      onSelect={() => undefined}
      parts={[
        { key: "discovery", label: "Discovery", value: 340, amount: "€340k" },
        { key: "proposal", label: "Proposal", value: 580, amount: "€580k" },
        {
          key: "negotiation",
          label: "Negotiation",
          value: 440,
          amount: "€440k",
        },
      ]}
    />
  ),
};

// Outcomes, not an order: each part wears its state's colour.
export const DonutOfOutcomes: Story = {
  render: () => (
    <DonutChart
      label="What happened to your handoffs"
      total="6"
      totalLabel="Total"
      parts={[
        {
          key: "accepted",
          label: "Accepted",
          value: 3,
          amount: "3",
          tone: "success",
        },
        {
          key: "recycled",
          label: "Sent back",
          value: 1,
          amount: "1",
          tone: "warning",
        },
        {
          key: "rejected",
          label: "Rejected",
          value: 1,
          amount: "1",
          tone: "danger",
        },
        {
          key: "waiting",
          label: "Not decided yet",
          value: 1,
          amount: "1",
          tone: "neutral",
        },
      ]}
    />
  ),
};

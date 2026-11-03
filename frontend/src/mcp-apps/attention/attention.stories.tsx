import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { atRiskFixture, commitmentsFixture, slippingFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Attention",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Slipping: Story = { args: { data: slippingFixture.data } };

export const AtRisk: Story = { args: { data: atRiskFixture.data } };

export const OpenPromises: Story = { args: { data: commitmentsFixture.data } };

/** A week where nothing slips says so. */
export const NothingSlipping: Story = { args: { data: { deals: [] } } };

/** The same card under the dark tokens. */
export const SlippingDark: Story = {
  args: { data: slippingFixture.data },
  globals: { theme: "dark" },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProviderMark } from "./provider-mark";

const meta = {
  title: "Components/Images and icons/Provider mark",
  component: ProviderMark,
} satisfies Meta<typeof ProviderMark>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Dispact: Story = { args: { providerKey: "dispact" } };
export const Slack: Story = { args: { providerKey: "slack" } };
export const Teams: Story = { args: { providerKey: "teams" } };

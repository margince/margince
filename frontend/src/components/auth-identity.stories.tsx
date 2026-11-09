import type { Meta, StoryObj } from "@storybook/react-vite";
import "./auth-core.css";
import { Wordmark } from "./auth-identity";

const meta: Meta<typeof Wordmark> = {
  title: "Signed out/Wordmark",
  component: Wordmark,
  args: { alt: "Margince" },
};
export default meta;
type Story = StoryObj<typeof Wordmark>;
export const Default: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };

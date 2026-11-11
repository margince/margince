// The answer to "this was refused, now what": send this one message anyway,
// with a reason. A rep the server offers nothing sees the review reference.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DirectSendAction } from "./directsendaction";

const meta: Meta<typeof DirectSendAction> = {
  title: "Patterns/Direct send action",
  component: DirectSendAction,
  decorators: [
    (Story) => (
      <QueryClientProvider client={new QueryClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DirectSendAction>;

// A rep who may overrule the engine. Danger-variant, because pressing it ends
// in a message going out against a refusal.
export const CanSendAnyway: Story = {
  args: {
    review: {
      reviewId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0001",
      actions: ["direct_send"],
      refusals: [],
    },
  },
};

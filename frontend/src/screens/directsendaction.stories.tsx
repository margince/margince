// The two answers to "this was refused, now what", and who sees which.
//
// A rep who holds the authority is offered the send. One who does not is
// offered the ask, which is a different component — so this one draws nothing,
// and that absence is the story worth looking at.

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
    },
  },
};

// A rep who may not. They are offered the ask elsewhere; here there is nothing
// to draw, and a disabled button would be a question they have to answer before
// they can ignore it.
export const OffersNothing: Story = {
  args: {
    review: {
      reviewId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0002",
      actions: ["request_decision"],
    },
  },
};

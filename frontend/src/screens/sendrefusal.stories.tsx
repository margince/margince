// Every way a send is refused, in the words the rep actually reads.
//
// The consent refusal is the one that changed: it used to end at a link to the
// contact page, where there is nothing to change, and now it offers the move
// the server says this rep may make.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SendRefusal } from "./sendrefusal";
import type { SendReview } from "./sendreview";

const review: SendReview = {
  reviewId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0001",
  actions: ["request_decision"],
};

const meta: Meta<typeof SendRefusal> = {
  title: "Patterns/Send refusal",
  component: SendRefusal,
  decorators: [
    (Story) => (
      <QueryClientProvider client={new QueryClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SendRefusal>;

// The engine refused, and this rep may ask somebody who can override it.
export const ConsentWithAnAsk: Story = {
  args: { refusal: "consent", personId: "p-1", review },
};

// The same refusal where the server offers nothing — an agent, or a message
// with nothing held to decide about. The reference is what remains.
export const ConsentWithOnlyAReference: Story = {
  args: {
    refusal: "consent",
    personId: "p-1",
    review: { ...review, actions: [] },
  },
};

// A mailbox connected before this product could send holds a read-only grant,
// and the provider will not widen one in place.
export const Mailbox: Story = { args: { refusal: "mailbox" } };

// A message carrying an unsubscribe link carries ONE recipient's credential,
// so it may only ever have one addressee.
export const SharedUnsubscribe: Story = {
  args: { refusal: "sharedUnsubscribe" },
};

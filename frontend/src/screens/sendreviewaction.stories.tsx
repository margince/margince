// What a rep is offered when the engine refuses their send.
//
// The three states are here because each is a different answer to the same
// question — "what now?" — and only the server decides which one a given rep
// sees. A reviewer's queue card, a dead end with a reference, and an
// installation whose client is older than its server.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { SendReview } from "./sendreview";
import { ReviewReference, SendReviewActions } from "./sendreviewaction";

const review: SendReview = {
  reviewId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0001",
  actions: ["request_decision"],
};

const meta: Meta<typeof SendReviewActions> = {
  title: "Patterns/Send review",
  component: SendReviewActions,
  decorators: [
    (Story) => (
      <QueryClientProvider client={new QueryClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SendReviewActions>;

// The ordinary case: a rep who cannot override the engine is offered the move
// they can make, which is asking somebody who can.
export const CanAsk: Story = { args: { review } };

// An agent, or a refusal with no message to decide about. The server offers
// nothing, so no button is drawn — a control nobody can use is a question the
// reader has to answer before they can ignore it.
export const NothingToOffer: Story = {
  args: { review: { ...review, actions: [] } },
};

// The reference on its own, which is what a rep keeps when there is no action:
// after a failed ask, and when the server named an action this build cannot
// draw. Both promises in the copy lean on it being on screen.
export const ReferenceOnly: StoryObj<typeof ReviewReference> = {
  render: () => <ReviewReference review={review} />,
};

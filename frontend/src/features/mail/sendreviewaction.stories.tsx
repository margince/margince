// What a rep is offered when the engine refuses their send.
//
// Each state is a different answer to "what now?". The server decides which one
// a given rep sees: the overrule, nothing to offer, or the reference alone.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { SendReview } from "./sendreview";
import { ReviewReference, SendReviewActions } from "./sendreviewaction";

const review: SendReview = {
  reviewId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0001",
  actions: ["overrule"],
  overrulable: [
    {
      contactId: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0002",
      category: "marketing",
    },
  ],
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

// The ordinary case: a rep who cannot direct a send overrules a machine refusal
// with their own reason.
export const CanOverrule: Story = { args: { review } };

// An agent, or a refusal with no message to decide about. The server offers
// nothing, so no button is drawn — a control nobody can use is a question the
// reader has to answer before they can ignore it.
export const NothingToOffer: Story = {
  args: { review: { ...review, actions: [] } },
};

// The reference on its own, which a rep keeps when there is no action. That
// happens after a failed overrule, and when the server named an action this
// build cannot draw.
export const ReferenceOnly: StoryObj<typeof ReviewReference> = {
  render: () => <ReviewReference review={review} />,
};

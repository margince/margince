// The last thing somebody sees before an exception is recorded in their name.
//
// The states here are the ones that decide whether the record is honest: the
// acknowledgement unticked, the reason empty, and the server's own caution
// rendered rather than ours.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { DirectSendModal } from "./directsendmodal";

const review: components["schemas"]["CommunicationReview"] = {
  id: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0001",
  state: "needs_repair",
  kind: "single",
  reason_code: "no_marketing_consent",
  refusals: [],
  warning: {
    version: "override-v1",
    text:
      "The engine refused this message, and that refusal stands: sending it now records an " +
      "exception, never consent. Your name, your reason and the exact message are written to a " +
      "record that cannot be edited afterwards. This covers this message alone — the next one to " +
      "the same recipient is judged from scratch.",
  },
};

const meta: Meta<typeof DirectSendModal> = {
  title: "Patterns/Direct send",
  component: DirectSendModal,
  decorators: [
    (Story) => (
      <QueryClientProvider client={new QueryClient()}>
        <Story />
      </QueryClientProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DirectSendModal>;

// As it opens: the caution read, nothing ticked, nothing typed. Confirm is
// refused and says which half is missing.
export const AsItOpens: Story = {
  args: { open: true, onClose: () => {}, review },
};

// A review whose server did not serve a caution — an older installation, or a
// contract this client is ahead of. The modal renders no warning rather than
// composing one, because inventing the words would record an acknowledgement of
// text nobody published.
export const WithNoServedWarning: Story = {
  args: {
    open: true,
    onClose: () => {},
    review: { ...review, warning: undefined },
  },
};

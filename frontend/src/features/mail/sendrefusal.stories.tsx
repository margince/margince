// The send refusals that are not about the recipients. A refusal about a
// recipient is drawn by the one send status: see "Patterns/Send permission".

import type { Meta, StoryObj } from "@storybook/react-vite";
import { SendRefusal } from "./sendrefusal";

const meta: Meta<typeof SendRefusal> = {
  title: "Patterns/Send refusal",
  component: SendRefusal,
};
export default meta;

type Story = StoryObj<typeof SendRefusal>;

// A mailbox connected before this product could send holds a read-only grant,
// and the provider will not widen one in place.
export const Mailbox: Story = { args: { refusal: "mailbox" } };

// A message carrying an unsubscribe link carries ONE recipient's credential,
// so it may only ever have one addressee.
export const SharedUnsubscribe: Story = {
  args: { refusal: "sharedUnsubscribe" },
};

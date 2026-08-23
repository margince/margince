// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { DealEmailAside } from "./dealemail";

// The box has exactly two states and always offers to write. Both are here, so
// the difference between them can be judged without arranging a buyer, a mail
// and a silence on a running stack.

const meta: Meta<typeof DealEmailAside> = {
  title: "Screens/Deal email box",
  component: DealEmailAside,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DealEmailAside>;

const DEAL = "deal-1";

// Answers the card read the box makes. A story that hit the real API would
// show whatever that installation happens to hold, which is not a state
// anybody chose to review.
function Served({
  replyTo,
  children,
}: Readonly<{ replyTo: string | null; children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["deal-status", DEAL], {
    deal_id: DEAL,
    story: { sentences: [] },
    reply_to: replyTo,
    generated_at: "2026-08-24T09:00:00Z",
    generated_by: "deterministic",
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

// The buyer wrote and nobody has answered: the mail continues their thread.
export const AnswerIsOwed: Story = {
  render: () => (
    <Served replyTo="activity-1">
      <DealEmailAside dealId={DEAL} />
    </Served>
  ),
};

// Nothing to answer — they never wrote, or their last message is answered
// already. The box still offers to write; the mail just starts a thread.
export const NothingToAnswer: Story = {
  render: () => (
    <Served replyTo={null}>
      <DealEmailAside dealId={DEAL} />
    </Served>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { RecordEmailAside } from "./recordemail";

// The box has exactly two states and always offers to write. Both are here, so
// the difference between them can be judged without arranging a caller that
// knows a thread is owed. dealemail.stories.tsx covers the deal-specific
// wording; this is the generic box a person or lead page mounts unstyled.

const meta: Meta<typeof RecordEmailAside> = {
  title: "Records/Email box",
  component: RecordEmailAside,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof RecordEmailAside>;

const PERSON = "person-1";

function withClient(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

// No reply target: the box offers a fresh mail to the record's contacts.
export const NothingToAnswer: Story = {
  render: () =>
    withClient(<RecordEmailAside entityType="person" entityId={PERSON} />),
};

// A reply target is supplied: the box offers to continue that thread.
export const AnswerIsOwed: Story = {
  render: () =>
    withClient(
      <RecordEmailAside
        entityType="person"
        entityId={PERSON}
        replyTo="activity-1"
      />,
    ),
};

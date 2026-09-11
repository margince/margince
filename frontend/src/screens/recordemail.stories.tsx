// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CalendarDays, MessageCircle, Phone } from "lucide-react";
import type { ReactNode } from "react";
import { IconAction } from "../design-system/iconaction";
import { LocaleProvider } from "../i18n";
import { EmailVerb, RecordEmailAside, RecordEmailVerb } from "./recordemail";

// The box has exactly two states and always offers to write. Both are here, so
// the difference between them can be judged without arranging a caller that
// knows a thread is owed. dealemail.stories.tsx covers the deal-specific
// wording; this is the generic box a person or lead page mounts unstyled.
//
// The header VERB is here too, and it is the same write on a different shape: a
// square whose envelope is its whole label. Judging it needs the neighbours it
// sits beside, so the verb stories draw the row rather than the button alone —
// what a reader has to be able to tell at a glance is which of these squares is
// the one the page is for, and whether the refused one still says why.

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

// The header verb, in the row it lives in. Square because an envelope is a verb
// a reader already knows from the glyph, and named on hover and to a screen
// reader by the one `label` prop.
export const WriteVerb: Story = {
  render: () =>
    withClient(
      <div className="record-actions">
        <EmailVerb onClick={() => {}} />
        <IconAction
          label="Call"
          icon={<Phone size={15} aria-hidden="true" />}
          onClick={() => {}}
        />
        <IconAction
          label="Meetings"
          icon={<CalendarDays size={15} aria-hidden="true" />}
          onClick={() => {}}
        />
      </div>,
    ),
};

// A page whose composer opens on another transport hands in its own label and
// glyph — the name a reader hears has to be the message they are about to send.
export const WriteVerbOnAnotherTransport: Story = {
  render: () =>
    withClient(
      <EmailVerb
        label="Send a WhatsApp"
        icon={<MessageCircle size={15} aria-hidden="true" />}
        onClick={() => {}}
      />,
    ),
};

// Refused, which is the state a square most needs to keep working: the reason
// reaches the control, so the tip and the line under it both say why rather
// than leaving a dead glyph the reader can only guess at.
export const WriteVerbRefused: Story = {
  render: () =>
    withClient(
      <EmailVerb
        onClick={() => {}}
        reason="This contact has no address on file."
      />,
    ),
};

// The same verb carrying its own composer, for a record page that keeps no
// composer state — the deal, the lead and the project all mount this one.
export const WriteVerbWithOwnComposer: Story = {
  render: () =>
    withClient(<RecordEmailVerb entityType="deal" entityId="deal-1" />),
};

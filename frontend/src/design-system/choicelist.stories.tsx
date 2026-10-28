// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Calendar, Globe, Mail, Server, Webhook } from "lucide-react";
import { useState } from "react";
import { Card } from "./atoms";
import { ChoiceList } from "./choicelist";
import { ProviderMark } from "./provider-mark";

// One question, every answer readable at rest , the control a binary decision
// needs and a dropdown cannot be. The cases on this canvas are the three a
// caller actually meets: a live either/or, a question nobody has answered yet,
// and a group a reader may look at but not change.

const meta: Meta<typeof ChoiceList> = {
  title: "Components/Forms and input/Choice list",
  component: ChoiceList,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ChoiceList>;

const EVERYONE_EXCEPT = {
  value: "everyone_except" as const,
  label: "Everyone I talk to , except the contacts I leave out",
  description:
    "Every conversation goes into the CRM until you name somebody to leave out.",
};
const ONLY_CHOSEN = {
  value: "only_chosen" as const,
  label: "Only the contacts I choose",
  description: "Nothing goes in until you name somebody.",
};
const REACH = [EVERYONE_EXCEPT, ONLY_CHOSEN];

// Stateful, because a radio group that never moves is the one thing this
// control must not be caught doing: the story IS the interaction.
function Live({
  initial,
}: Readonly<{ initial: "" | "everyone_except" | "only_chosen" }>) {
  const [value, setValue] = useState<"" | "everyone_except" | "only_chosen">(
    initial,
  );
  return (
    <ChoiceList
      legend="Which conversations go into the CRM?"
      value={value}
      choices={REACH}
      onChange={setValue}
    />
  );
}

export const Answered: Story = {
  render: () => <Live initial="only_chosen" />,
};

// The unanswered group: no option carries the dot, so nothing claims to be the
// default. A question with a pre-selected answer is a question somebody can save
// without ever having read it.
export const Unanswered: Story = {
  render: () => <Live initial="" />,
};

// A reader who may not change it. The words stay legible: both answers can be
// read, as true for somebody who cannot pick one.
export const Disabled: Story = {
  args: {
    legend: "Which conversations go into the CRM?",
    value: "everyone_except",
    choices: REACH,
    disabled: true,
    onChange: () => {},
  },
};

// One answer refused while the other stays open, with the reason as its help
// line, so the reader sees what the refused answer would do and why not yet.
export const OneAnswerRefused: Story = {
  args: {
    legend: "Which conversations go into the CRM?",
    value: "only_chosen",
    choices: [
      {
        ...EVERYONE_EXCEPT,
        disabled: true,
        description: "Connect a mailbox first.",
      },
      ONLY_CHOSEN,
    ],
    onChange: () => {},
  },
};

// The legend off screen, for a card whose own heading already asks the question.
// It still names the group for a screen reader.
export const HiddenLegend: Story = {
  args: {
    legend: "Which conversations go into the CRM?",
    hideLegend: true,
    value: "only_chosen",
    choices: REACH,
    onChange: () => {},
  },
};

// A label with no description, which is the short form: the type carries the
// hierarchy, so an option that needs no explanation simply has none.
export const LabelsOnly: Story = {
  args: {
    legend: "Send the digest",
    value: "weekly",
    choices: [
      { value: "daily", label: "Every morning" },
      { value: "weekly", label: "Monday mornings only" },
    ],
    onChange: () => {},
  },
};

const SERVICES = [
  {
    value: "mail" as const,
    label: "Mail",
    description: "Conversations with your contacts go into the CRM.",
    mark: <Mail aria-hidden="true" />,
  },
  {
    value: "calendar" as const,
    label: "Calendar",
    description: "Not available for an IMAP mailbox.",
    mark: <Calendar aria-hidden="true" />,
    disabled: true,
  },
];

function ManyLive() {
  const [value, setValue] = useState<("mail" | "calendar")[]>(["mail"]);
  return (
    <ChoiceList
      legend="What should Margince read?"
      multiple
      layout="cards"
      value={value}
      choices={SERVICES}
      onChange={setValue}
    />
  );
}

// Several answers at once: checkbox plates in one group, one refused with its
// reason. The picks come back in the order the choices are listed.
export const ManyAsCards: Story = { render: () => <ManyLive /> };
export const ManyAsCardsDark: Story = {
  ...ManyAsCards,
  globals: { theme: "dark" },
};

const DAYS = [
  { value: "mon", label: "Mon" },
  { value: "tue", label: "Tue" },
  { value: "wed", label: "Wed" },
  { value: "thu", label: "Thu" },
  { value: "fri", label: "Fri" },
  { value: "sat", label: "Sat" },
  { value: "sun", label: "Sun" },
];

function DaysLive() {
  const [value, setValue] = useState<string[]>(["mon", "tue", "wed"]);
  return (
    <ChoiceList
      legend="Bookable days"
      multiple
      layout="row"
      value={value}
      choices={DAYS}
      onChange={setValue}
    />
  );
}

// Pick-several in one line: short labels read as a range, so a column would
// spend seven rows on one question.
export const MultipleRow: Story = { render: () => <DaysLive /> };

const VENDORS = [
  {
    value: "google" as const,
    label: "Google",
    description: "Gmail and Google Calendar.",
    mark: <ProviderMark providerKey="google" />,
  },
  {
    value: "microsoft" as const,
    label: "Microsoft",
    description: "Outlook and Microsoft 365 calendar.",
    mark: <ProviderMark providerKey="microsoft" />,
  },
  {
    value: "imap" as const,
    label: "Other mailbox (IMAP)",
    description: "Mail only, no calendar.",
    mark: <ProviderMark providerKey="imap" />,
  },
];

function VendorsLive() {
  const [value, setValue] = useState<"" | "google" | "microsoft" | "imap">(
    "google",
  );
  return (
    <ChoiceList
      legend="Where is the account?"
      layout="cards"
      value={value}
      choices={VENDORS}
      onChange={setValue}
    />
  );
}

// One answer as plates with the vendor's own mark, on an elevated card as in a
// drawer. Every live plate stays light and the chosen one is drawn selected.
export const SingleCardsWithMark: Story = {
  render: () => (
    <Card as="div">
      <VendorsLive />
    </Card>
  ),
};
export const SingleCardsWithMarkDark: Story = {
  ...SingleCardsWithMark,
  globals: { theme: "dark" },
};

const SOURCES = [
  {
    value: "site" as const,
    label: "Your website",
    description: "A form on your own pages.",
    mark: <Globe aria-hidden="true" />,
    group: "No developer needed",
  },
  {
    value: "server" as const,
    label: "Website server",
    description: "Your server forwards each enquiry, signed.",
    mark: <Server aria-hidden="true" />,
    group: "For developers",
  },
  {
    value: "webhook" as const,
    label: "Webhook",
    description: "A webhook made with a builder's API.",
    mark: <Webhook aria-hidden="true" />,
    group: "For developers",
  },
];

function GroupedLive() {
  const [value, setValue] = useState<"" | "site" | "server" | "webhook">(
    "site",
  );
  return (
    <ChoiceList
      legend="Where is your form?"
      layout="cards"
      value={value}
      choices={SOURCES}
      onChange={setValue}
    />
  );
}

// Plates under a label per kind. Each group fills the width on its own, so a
// group of two never leaves an empty column beside a group of one or three.
export const GroupedCards: Story = { render: () => <GroupedLive /> };
export const GroupedCardsDark: Story = {
  ...GroupedCards,
  globals: { theme: "dark" },
};

function LinkLive() {
  const [value, setValue] = useState<"classified" | "held" | "shared">(
    "classified",
  );
  return (
    <ChoiceList
      legend="How is mail kept?"
      value={value}
      onChange={setValue}
      choices={[
        {
          value: "classified",
          label: "Sorted by Margince",
          description: "Mail about your contacts goes in, the rest stays out.",
        },
        { value: "held", label: "Held for review" },
        {
          value: "shared",
          label: "Shared with the team",
          disabled: true,
          description: (
            <>
              Needs a workspace opt-in.{" "}
              <a className="link-button" href="#opt-in">
                Ask an admin
              </a>
            </>
          ),
        },
      ]}
    />
  );
}

// A refused answer whose description links to what unlocks it. Keep it to one
// short link: its words join the answer's accessible name.
export const DescriptionWithLink: Story = { render: () => <LinkLive /> };

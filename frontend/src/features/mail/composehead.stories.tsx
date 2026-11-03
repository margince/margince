// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../testing/story-utils";
import { AddressBlock } from "./composehead";

// The mail head's address block: To and Cc standing, Bcc a button until it is
// asked for, and — under the addresses, because it is about the ones standing
// there — the notice that one of them is bouncing.

// A named address draws as its name and avatar; its card holds the address,
// the title and the account.
const CONTACTS = [
  {
    value: "ada@brandt.example",
    label: "Ada Brandt-Sachsinger",
    hint: "ada@brandt.example",
    card: {
      identity: "c-ada",
      href: "#/contacts/c-ada",
      about: "Head of Logistics · Brandt Logistik",
    },
  },
  {
    value: "lars@brandt.example",
    label: "Lars Strojny",
    hint: "lars@brandt.example",
    card: { identity: "c-lars", about: "Brandt Logistik" },
  },
  {
    value: "luitpold@brandt.example",
    label: "Luitpold Alexander Zollorsch",
    hint: "luitpold@brandt.example",
    card: { identity: "c-luitpold", about: "CFO · Brandt Logistik" },
  },
];

/**
 * The block owns no state, so the story lends it some: a reader in the catalog
 * can add and remove tokens and watch Bcc open, which is the half of this
 * component a static frame cannot show.
 */
function Head({
  start = ["ada@brandt.example"],
  deadRecipients = [],
  invalidTo = false,
}: Readonly<{
  start?: string[];
  deadRecipients?: readonly string[];
  invalidTo?: boolean;
}>) {
  const [to, setTo] = useState<string[]>(start);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
  const [copiesOpen, setCopiesOpen] = useState(false);
  return (
    <StoryProviders>
      <AddressBlock
        to={to}
        onToChange={setTo}
        cc={cc}
        onCcChange={setCc}
        bcc={bcc}
        onBccChange={setBcc}
        copiesOpen={copiesOpen}
        onOpenCopies={() => setCopiesOpen(true)}
        suggestions={CONTACTS}
        onToEditing={() => {}}
        invalidTo={invalidTo}
        needTo="Name at least one recipient."
        deadRecipients={deadRecipients}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof AddressBlock> = {
  title: "Patterns/Compose mail/Address block",
  component: AddressBlock,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof AddressBlock>;

/** The ordinary head: one recipient, no Cc, Bcc still a button. */
export const Default: Story = { render: () => <Head /> };

/** Three recipients: the first row stays on the label's line, the rest wrap. */
export const ManyRecipients: Story = {
  render: () => (
    <Head
      start={[
        "luitpold@brandt.example",
        "lars@brandt.example",
        "ada@brandt.example",
        "someone@else.example",
      ]}
    />
  ),
};

/** The To line a send would refuse, with the need beside the field. */
export const RecipientMissing: Story = {
  render: () => <Head invalidTo />,
};

/**
 * An address the ledger knows does not arrive. A warning and not a refusal —
 * the rep may know something the ledger does not — so the heading states the
 * claim and the body says what to do about it.
 */
export const RecipientBouncing: Story = {
  render: () => <Head deadRecipients={["ada@brandt.example"]} />,
};

/** The same notice in dark, where the tone reaches the heading's ink. */
export const RecipientBouncingDark: Story = {
  globals: { theme: "dark" },
  render: () => <Head deadRecipients={["ada@brandt.example"]} />,
};

export const Narrow: Story = {
  ...Default,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

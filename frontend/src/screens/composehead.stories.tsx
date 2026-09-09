// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { AddressBlock } from "./composehead";
import { StoryProviders } from "./story-utils";

// The mail head's address block: To and Cc standing, Bcc a button until it is
// asked for, and — under the addresses, because it is about the ones standing
// there — the notice that one of them is bouncing.

/**
 * The block owns no state, so the story lends it some: a reader in the catalog
 * can add and remove tokens and watch Bcc open, which is the half of this
 * component a static frame cannot show.
 */
function Head({
  deadRecipients = [],
  invalidTo = false,
}: Readonly<{
  deadRecipients?: readonly string[];
  invalidTo?: boolean;
}>) {
  const [to, setTo] = useState<string[]>(["ada@brandt.example"]);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
  const [bccOpen, setBccOpen] = useState(false);
  return (
    <StoryProviders>
      <AddressBlock
        to={to}
        onToChange={setTo}
        cc={cc}
        onCcChange={setCc}
        bcc={bcc}
        onBccChange={setBcc}
        bccOpen={bccOpen}
        onOpenBcc={() => setBccOpen(true)}
        suggestions={[]}
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

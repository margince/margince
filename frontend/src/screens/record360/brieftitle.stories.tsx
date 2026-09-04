// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { CallCard } from "./reading";

// The head of every card a machine wrote, on its own.
//
// It is one line making one claim, and the claim is carried by the MARK: the
// indigo tile means "a machine wrote what is under this" everywhere in the
// product. What the stories are for is the pairing — the tile has to read as
// authorship beside the record's name and never as a control sitting next to
// one — and the two ways the line ends: a record that has a name, and one this
// reader may not be told the name of.
//
// Check both themes. `--aiText` lifts on dark while the tile's ground stays
// the same translucent indigo, so the mark's contrast is the thing to look at
// twice.

// The head in the card it heads, because that is the only place it is read:
// its own claim has to hold against the verdict word directly under it, which
// is the loudest thing on the record page.
function Head({ name }: Readonly<{ name?: string }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <CallCard
          name={name}
          standing={{ label: "Good", tone: "calm" }}
          because="They replied within a day, and nothing is owed."
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof CallCard> = {
  title: "Records/Company 360/Brief title",
  component: CallCard,
};
export default meta;

type Story = StoryObj<typeof CallCard>;

export const Named: Story = {
  render: () => <Head name="Kugellager-online.de" />,
};

// A long name is the record's, not the head's, to shorten. The line is what
// has to hold: the name keeps the mark's company and the "· 360" stays part
// of it, however many words the record was registered under.
export const LongName: Story = {
  render: () => (
    <Head name="Apartment Management Services Hamburg-Altona GmbH & Co. KG" />
  ),
};

// No name reached this reader — a withheld record, or a read that failed
// before the organisation came back. The card still says what it is a reading
// of, in words that do not pretend to know which account it was.
export const Unnamed: Story = { render: () => <Head /> };

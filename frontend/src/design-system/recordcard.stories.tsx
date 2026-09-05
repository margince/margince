// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Badge } from "./atoms";
import { AvatarStack } from "./avatarstack";
import { RecordCard } from "./recordcard";

// A record listed somewhere else. The stories are the states a caller
// actually meets: the two kinds, a record with nothing but a name, one the
// reader may not open, and the stack they arrive in.
const meta: Meta<typeof RecordCard> = {
  title: "Design System/RecordCard",
  component: RecordCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 420 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof RecordCard>;

/** A contact on the account that lists them: what they do, and how to write. */
export const Person: Story = {
  render: () => (
    <RecordCard
      kind="person"
      name="Anna Brandt"
      identity="p-1"
      href="#/contacts/p-1"
      position="Head of Procurement"
      email="anna.brandt@nordwind-logistik.de"
    />
  ),
};

/**
 * A company, which the mark's shape says before the name is read. The two
 * shapes are the reason `kind` exists: on a page carrying both, the square is
 * what tells a reader which chips are companies before they read a word.
 */
export const Organization: Story = {
  render: () => (
    <RecordCard
      kind="organization"
      name="Nordwind Logistik GmbH"
      identity="o-1"
      href="#/companies/o-1"
      position="Freight forwarding · Hamburg"
    />
  ),
};

/**
 * A record the surface knows only the name of. The card does not reserve the
 * space its missing facts would have taken — a stack of cards with a hole in
 * each reads as a page that failed to load.
 */
export const NameOnly: Story = {
  render: () => (
    <RecordCard
      kind="person"
      name="Jonas Weiß"
      identity="p-2"
      href="#/contacts/p-2"
    />
  ),
};

/**
 * What the SURFACE knows, rather than the record: which colleagues already
 * reach this contact, or that an employment is the current one. It keeps its
 * own track, so a long name ellipsises before it arrives.
 */
export const WithAside: Story = {
  render: () => (
    <ul className="record-card-list">
      <li>
        <RecordCard
          kind="person"
          name="Anna Brandt"
          identity="p-1"
          href="#/contacts/p-1"
          position="Head of Procurement"
          email="anna.brandt@nordwind-logistik.de"
          aside={
            <AvatarStack
              people={[{ name: "Tim Rasche" }, { name: "Lena Ott" }]}
            />
          }
        />
      </li>
      <li>
        <RecordCard
          kind="person"
          name="Maximilian von Hohenlohe-Schillingsfürst"
          identity="p-3"
          href="#/contacts/p-3"
          position="Geschäftsführer Einkauf und Logistik"
          email="maximilian.von.hohenlohe@nordwind-logistik.de"
          aside={<Badge tone="accent">Champion</Badge>}
        />
      </li>
    </ul>
  ),
};

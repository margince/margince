// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Link as LinkIcon, Mail, MapPin, Phone } from "lucide-react";
import { LocaleProvider } from "../i18n";
import { IdentityFact, IdentityLine, IdentityMeta } from "./identityline";

// The facts under a record's name. Every record page draws this row; the two
// stories below are the two shapes it comes in, which is the whole of its API.
const meta: Meta<typeof IdentityLine> = {
  title: "Design System/IdentityLine",
  component: IdentityLine,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof IdentityLine>;

/**
 * An ACCOUNT's line: clauses of one sentence about the record, strung on dots.
 * The last fact qualifies the record rather than being one more thing it is,
 * so it steps back inside the line.
 */
export const Dotted: Story = {
  render: () => (
    <IdentityMeta>
      <IdentityLine>
        <IdentityFact>nordwind-logistik.de</IdentityFact>
        <IdentityFact>Hamburg</IdentityFact>
        <IdentityFact>Freight forwarding</IdentityFact>
        <IdentityFact>201–500 employees</IdentityFact>
        <IdentityFact quiet>Owner: Tim Rasche</IdentityFact>
      </IdentityLine>
    </IdentityMeta>
  ),
};

/**
 * A CONTACT's line: each fact is a separate handle on the person rather than a
 * clause about them, so whitespace tells them apart instead of a dot.
 */
export const Spaced: Story = {
  render: () => (
    <IdentityMeta>
      <IdentityLine separator="space">
        <IdentityFact icon={<Mail size={13} aria-hidden="true" />}>
          mareike.vollmer@nordwind-logistik.de
        </IdentityFact>
        <IdentityFact icon={<Phone size={13} aria-hidden="true" />}>
          +49 40 3311 8842
        </IdentityFact>
        <IdentityFact icon={<MapPin size={13} aria-hidden="true" />}>
          Hamburg
        </IdentityFact>
        <IdentityFact icon={<LinkIcon size={13} aria-hidden="true" />}>
          LinkedIn
        </IdentityFact>
        <IdentityFact quiet>Buying role: Decision maker</IdentityFact>
      </IdentityLine>
    </IdentityMeta>
  ),
};

/**
 * The case the component exists for: a record that does not know something
 * renders nothing for it, and the line closes over the gap rather than showing
 * a doubled dot or opening with one. Three of the five facts here are absent.
 */
export const FactsTheRecordDoesNotHave: Story = {
  render: () => {
    // Typed rather than inferred: the absent facts are `string | undefined`
    // on a real record, and a literal that infers them as `undefined` is a
    // different shape from the one a caller actually holds.
    const record: {
      website?: string;
      city?: string;
      industry?: string;
      size?: string;
      owner: string;
    } = { city: "Hamburg", owner: "Tim Rasche" };
    return (
      <IdentityMeta>
        <IdentityLine>
          {record.website && <IdentityFact>{record.website}</IdentityFact>}
          {record.city && <IdentityFact>{record.city}</IdentityFact>}
          {record.industry && <IdentityFact>{record.industry}</IdentityFact>}
          {record.size && <IdentityFact>{record.size}</IdentityFact>}
          <IdentityFact quiet>Owner: {record.owner}</IdentityFact>
        </IdentityLine>
      </IdentityMeta>
    );
  },
};

/**
 * Two lines: what the record IS, then when its row was written. A record with
 * two different things to say under its name says them as two answers rather
 * than as one sentence a reader has to re-parse.
 */
export const TwoLines: Story = {
  render: () => (
    <IdentityMeta>
      <IdentityLine>
        <IdentityFact>Hamburg</IdentityFact>
        <IdentityFact>Freight forwarding</IdentityFact>
        <IdentityFact quiet>Owner: Tim Rasche</IdentityFact>
      </IdentityLine>
      <IdentityLine>
        <IdentityFact quiet>Created 1 June 2026 by Jo Ziethen</IdentityFact>
      </IdentityLine>
    </IdentityMeta>
  ),
};

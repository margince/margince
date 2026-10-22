// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StandingDims } from "./standingdims";

const meta: Meta<typeof StandingDims> = {
  title: "Components/Status indicators/Standing dims",
  component: StandingDims,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "22rem" }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof StandingDims>;

/** A company's three health dimensions, one of them not rated. */
export const Company: Story = {
  args: {
    label: "Health",
    dims: [
      {
        key: "relationship",
        label: "Relationship",
        value: "Good",
        sub: "Last met them 12 days ago",
        tone: "warning",
      },
      {
        key: "commercial",
        label: "Commercial",
        value: "Not rated",
        sub: "No open deal",
        tone: "neutral",
      },
      {
        key: "payment",
        label: "Payment",
        value: "Strong",
        sub: "4 of 4 paid on time",
        tone: "success",
      },
    ],
  },
};

/** A project with an overdue promise. */
export const Project: Story = {
  args: {
    dims: [
      {
        key: "commitments",
        label: "Commitments",
        value: "1 overdue",
        sub: "of 3 open · kickoff agenda",
        tone: "danger",
      },
      {
        key: "time",
        label: "Time used",
        value: "Day 166",
        sub: "of 274 · ends 30 Nov",
        tone: "success",
      },
      {
        key: "deals",
        label: "Deals",
        value: "1 won of 2",
        sub: "phase two open",
        tone: "neutral",
      },
    ],
  },
};

/** A new record: every row says what it has not got. */
export const NothingYet: Story = {
  args: {
    dims: [
      { key: "a", label: "Relationship", value: "Not rated", tone: "neutral" },
      { key: "b", label: "Commercial", value: "Not rated", tone: "neutral" },
      { key: "c", label: "Payment", value: "Not connected", tone: "neutral" },
    ],
  },
};

/** Readings that count something open a card of what: hover, focus or a press.
 *  The line that holds the count wears the dotted mark. */
export const WithCards: Story = {
  args: {
    dims: [
      {
        key: "relationship",
        label: "Relationship",
        value: "Strong",
        sub: "3 contacts here are in touch.",
        tone: "success",
        peek: {
          on: "sub",
          head: "Contacts here who have been in touch",
          items: [
            {
              key: "a",
              title: "Sarah Cole",
              sub: "last in touch 22 Oct",
              href: "#/contacts/a",
            },
            {
              key: "b",
              title: "Nadia Ferreira",
              sub: "VP Operations · last in touch 12 Oct",
              href: "#/contacts/b",
            },
          ],
        },
      },
      {
        key: "exchange",
        label: "Exchange",
        value: "2 in · 2 out",
        sub: "Last 90 days",
        tone: "success",
        peek: {
          head: "Messages in the last 90 days, newest first",
          items: [
            { key: "m", title: "Re: ROI discussion", sub: "Received · 19 Oct" },
          ],
        },
      },
      {
        key: "payment",
        label: "Payment",
        value: "At risk",
        sub: "Payment is overdue.",
        tone: "danger",
      },
    ],
  },
};

/** German is longer: names and values wrap inside their column and the sub
 *  line keeps to two lines with its whole words on the tip. */
export const LongGermanReadings: Story = {
  decorators: [
    (Story) => (
      <div lang="de" style={{ maxWidth: "18rem" }}>
        <Story />
      </div>
    ),
  ],
  args: {
    label: "Gesundheit",
    dims: [
      {
        key: "relationship",
        label: "Beziehungsqualität",
        value: "Gefährdet",
        sub: "Seit über drei Wochen keine Antwort auf unser letztes Angebot",
        tone: "danger",
      },
      {
        key: "commercial",
        label: "Geschäftsentwicklung",
        value: "Nicht bewertet",
        tone: "neutral",
      },
    ],
  },
};

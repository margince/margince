// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BuyerFrame, BuyerHero, ContactCard, DeadLink } from "./buyerroomframe";
import { installFetchStub, StoryProviders } from "./story-utils";
import "./buyerroom.css";

// The page a client meets, without the board: the ground and our mark, the
// hero the seller wrote, the face beside the documents, and the screen a link
// lands on once it stops admitting anyone.
//
// No session and no rail — an outside contact holds a link and nothing else —
// so these stories route nothing but the stub that keeps the page off the
// network.

const WELCOME =
  "Welcome, Laura. Everything about the rollout lives here: the contract, " +
  "the plan and the two questions your team raised last week.";

// The refusal's sentence is the product's, not this story's: the screen builds
// it with `t()` from whichever refusal it met, so the catalog asks for the same
// key rather than typing English into the panel.
function Refused({ messageKey }: Readonly<{ messageKey: MessageKey }>) {
  const t = useT();
  return <DeadLink message={t(messageKey)} />;
}

function page(children: React.ReactNode) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <BuyerFrame>{children}</BuyerFrame>
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Signed out/Deal room frame",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

// A room still taking answers: the live pill, the seller's own welcome, and
// the steward named beside it — the answer to the question a buyer has after
// reading, which is whom to ask.
export const Live: Story = {
  render: page(
    <>
      <BuyerHero
        title="Acme rollout"
        welcome={WELCOME}
        access="live"
        closedAt={null}
      />
      <ContactCard stewardName="Ada Admin" access="live" />
    </>,
  ),
};

// Closed, and the day it was fixed said beside the word: "closed" on its own
// leaves a buyer wondering whether they missed something last week or last
// year.
export const Closed: Story = {
  render: page(
    <>
      <BuyerHero
        title="Acme rollout"
        welcome={WELCOME}
        access="closed"
        closedAt="2026-07-31T16:00:00Z"
      />
      <ContactCard stewardName="Ada Admin" access="closed" />
    </>,
  ),
};

// The steward's seat is gone. No mark and no name — a monogram built from the
// words "your contact" would draw a person who does not exist — so the card
// keeps only the sentence that still holds.
export const NoStewardLeft: Story = {
  render: page(
    <>
      <BuyerHero
        title="Acme rollout"
        welcome=""
        access="live"
        closedAt={null}
      />
      <ContactCard stewardName={null} access="live" />
    </>,
  ),
};

// A state this build has no claim to make about: the pill is omitted rather
// than guessed, because a room whose access word is newer than this client is
// not one we may describe.
export const AccessWordWeDoNotKnow: Story = {
  render: page(
    <BuyerHero
      title="Acme rollout"
      welcome={WELCOME}
      access="archived"
      closedAt={null}
    />,
  ),
};

// The link no longer admits anyone. Used, lapsed, retired or never valid all
// read the same, and the page offers the one recovery a buyer has.
export const LinkNoLongerWorks: Story = {
  render: page(<Refused messageKey="buyer.linkDead" />),
};

// The URL arrived with no token at all — a link that lost its query on the way
// through somebody's mail client. A different sentence, the same recovery.
export const NoLinkAtAll: Story = {
  render: page(<Refused messageKey="buyer.noLink" />),
};

// The same dead end in the dark theme, where the panel, the field and the page
// ground compress toward each other.
export const LinkNoLongerWorksDark: Story = {
  globals: { theme: "dark" },
  render: page(<Refused messageKey="buyer.linkDead" />),
};

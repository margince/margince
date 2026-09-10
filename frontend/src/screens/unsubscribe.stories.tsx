// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { UnsubscribeScreen } from "./unsubscribe";

// Where the VISIBLE unsubscribe link in an outgoing message lands. The page
// never withdraws anything on arrival — a mail scanner following the link is
// nobody pressing it — so what a reader meets first is the ask, and the states
// worth drawing are the ones they cannot press their way out of: a dead link,
// a purpose the catalog does not carry, and a read that was refused.
//
// It carries no session: the token in the URL is the whole capability, so no
// story here routes `GET /me`.

const TOKEN = "tok-123";

const CENTER = {
  masked_email: "m•••••@example.com",
  workspace_name: "Brandt Automotive",
  refused: [],
  purposes: [
    {
      key: "business_correspondence",
      label: "Direct correspondence",
      state: "unknown",
      locked: false,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: true,
    },
    {
      key: "transactional",
      label: "Deal & service messages",
      state: "granted",
      locked: true,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: false,
    },
  ],
};

const meta: Meta<typeof UnsubscribeScreen> = {
  title: "Signed out/Unsubscribe",
  component: UnsubscribeScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof UnsubscribeScreen>;

function Served({
  answer = () => jsonResponse(CENTER),
  children,
}: Readonly<{ answer?: () => Response; children: ReactNode }>) {
  installFetchStub({ [`GET /public/preferences/${TOKEN}`]: answer });
  return <StoryProviders>{children}</StoryProviders>;
}

/** The ask, which is the page nearly every reader sees: one kind of email, one
 * press, and what happens after it. */
export const TheAsk: Story = {
  render: () => (
    <Served>
      <UnsubscribeScreen token={TOKEN} purpose="business_correspondence" />
    </Served>
  ),
};

/** A purpose nobody may switch off. No button at all — a control that always
 * fails is worse than an absent one — and the reason instead. */
export const LockedPurpose: Story = {
  render: () => (
    <Served>
      <UnsubscribeScreen token={TOKEN} purpose="transactional" />
    </Served>
  ),
};

/** The link carried no purpose. A dead end is the page rather than a notice on
 * it, so the answer is the heading. */
export const DeadLink: Story = {
  render: () => (
    <Served>
      <UnsubscribeScreen token={TOKEN} />
    </Served>
  ),
};

/** The link names a kind of email this catalog does not carry, so the page
 * hands the reader their full preferences instead of reading as broken. */
export const UnknownPurpose: Story = {
  render: () => (
    <Served>
      <UnsubscribeScreen token={TOKEN} purpose="no_such_purpose" />
    </Served>
  ),
};

/**
 * Rate limited. A "not now" keeps its retry, because somebody who came here to
 * stop an email deserves a second press rather than a dead end.
 */
export const RateLimited: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ title: "slow down" }, 429)}>
      <UnsubscribeScreen token={TOKEN} purpose="business_correspondence" />
    </Served>
  ),
};

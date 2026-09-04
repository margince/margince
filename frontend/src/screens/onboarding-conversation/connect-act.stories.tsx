// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { ConnectAct } from "./connect-act";
import type { ProviderAvailability } from "./connect-scene";
import { initialConversationState } from "./conversation-machine";
import type {
  ConversationPhase,
  ConversationState,
} from "./conversation-types";

// The act that connects a mailbox, and offers LinkedIn beside it.
//
// The two are NOT the same kind of thing, and the stories keep them apart
// because the surface does: mail is the gate the act waits on, LinkedIn is a
// recommended addition whose own status resolves independently and never holds
// the act up. A story that showed them as one pair of switches would be
// documenting a screen where finishing depends on both.
//
// `outcome` and `returningProvider` are ROUTE segments, replayed by a stale
// bookmark or a reload of an old consent-return URL with no live attempt behind
// them — so a story for each is a story about a URL, not about a request.
//
// The eyebrow is the rail position, and neither half of it is ever grouped.

function state(
  phase: ConversationPhase,
  overrides: Partial<ConversationState> = {},
): ConversationState {
  return {
    ...initialConversationState,
    act: "connect",
    phase,
    ...overrides,
  };
}

// Every provider connectable, which is what an installation that finished its
// own setup looks like. A story that needs a blocked one says so.
const ALL_READY: ProviderAvailability[] = [
  { provider: "gmail", reason: "ready" },
  { provider: "graph", reason: "ready" },
  { provider: "imap", reason: "ready" },
];

function act(
  conversation: ConversationState,
  route: { outcome?: string; returningProvider?: string } = {},
  locale?: "de",
  providers: ProviderAvailability[] = ALL_READY,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ channel_connection: ["read", "create"] }),
      "GET /connectors": () => jsonResponse({ data: [], providers }),
    });
    return (
      <StoryProviders locale={locale}>
        <ConnectAct
          state={conversation}
          dispatch={() => {}}
          persist={async () => true}
          outcome={route.outcome}
          returningProvider={route.returningProvider}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectAct> = {
  title: "Onboarding/Conversation/Connect act",
  component: ConnectAct,
};
export default meta;
type Story = StoryObj<typeof ConnectAct>;

/** Nothing connected yet: the mail gate, and LinkedIn offered beside it. */
export const AwaitingConsent: Story = { render: act(state("cn.consent")) };

/** The German act — the copy that has to hold two different KINDS of offer on
 *  one surface without making them look alike. */
export const AwaitingConsentGerman: Story = {
  render: act(state("cn.consent"), {}, "de"),
};

/** LinkedIn connected while mail is still outstanding, which is the case that
 *  proves it is not a gate. */
export const LinkedinSettledMailPending: Story = {
  render: act(state("cn.consent", { linkedinStatus: "connected" })),
};

/** LinkedIn declined. The act still finishes on mail alone — a recommended
 *  addition that blocked the run would be a gate wearing a suggestion's
 *  clothes. */
export const LinkedinSkipped: Story = {
  render: act(state("cn.consent", { linkedinStatus: "skipped" })),
};

/** Back from consent, granted. */
export const ReturnedGranted: Story = {
  render: act(state("cn.consent"), {
    outcome: "connected",
    returningProvider: "gmail",
  }),
};

/**
 * Back from consent, refused. The act says what did not happen and offers the
 * door again rather than treating a refusal as a fault: somebody declining a
 * mailbox is an answer, not an error.
 */
export const ReturnedRefused: Story = {
  render: act(state("cn.consent"), {
    outcome: "denied",
    returningProvider: "gmail",
  }),
};

/**
 * A stale return URL: an outcome replayed with no live attempt behind it. The
 * act must not report a connection it has no evidence for, which is why the
 * route segments are treated as claims rather than as facts.
 */
export const StaleReturn: Story = {
  render: act(state("cn.consent"), {
    outcome: "connected",
    returningProvider: "unknown-provider",
  }),
};

/**
 * No Microsoft app registered for this organization. The card cannot be opened
 * because the connect behind it would be refused, so it says what is missing
 * and where it is fixed rather than leaving the reader to find that out by
 * clicking. Google and IMAP are unaffected, which is the point: this is one
 * vendor's configuration, not a broken screen.
 */
export const MicrosoftAppMissing: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_missing" },
    { provider: "imap", reason: "ready" },
  ]),
};

/**
 * A Microsoft app IS registered and its secret will not open. A different
 * sentence from the one above, and deliberately so: telling this operator that
 * no app exists sends them to register a second one.
 */
export const MicrosoftAppUnusable: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_unusable" },
    { provider: "imap", reason: "ready" },
  ]),
};

/**
 * A deployment that does not serve Microsoft at all. Nobody's setting, so the
 * card offers no link to a form that would have nothing in it.
 */
export const MicrosoftUnsupported: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "unsupported" },
    { provider: "imap", reason: "ready" },
  ]),
};

/** At 390px the two offers stack, and the one that gates has to stay legibly
 *  the primary of the pair. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: act(state("cn.consent")),
};

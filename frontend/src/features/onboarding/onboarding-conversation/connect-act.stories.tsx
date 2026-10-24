// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
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

/** Nothing connected yet: the account to pick, and LinkedIn below it. */
export const AwaitingConsent: Story = { render: act(state("cn.consent")) };

export const AwaitingConsentDark: Story = {
  render: act(state("cn.consent")),
  globals: { theme: "dark" },
};

/** The German act — the copy that has to hold two different KINDS of offer on
 *  one surface without making them look alike. */
export const AwaitingConsentGerman: Story = {
  render: act(state("cn.consent"), {}, "de"),
};

/** The LinkedIn profile saved while mail is still outstanding, which is the
 *  case that proves it is not a gate. */
export const LinkedinSettledMailPending: Story = {
  render: act(state("cn.consent", { linkedinStatus: "saved" })),
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
 * Back from a chained consent whose calendar leg was refused. The calendar
 * says it was not connected, and claims nothing about a mailbox the roster
 * does not show.
 */
export const ReturnedCalendarRefused: Story = {
  render: act(state("cn.consent"), {
    outcome: "denied",
    returningProvider: "graphcal",
  }),
};

export const ReturnedCalendarRefusedDark: Story = {
  render: act(state("cn.consent"), {
    outcome: "denied",
    returningProvider: "graphcal",
  }),
  globals: { theme: "dark" },
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
 * No Microsoft app registered for this company. The account says it waits for
 * an administrator and the others say they are open now. Picking it shows who
 * fixes it rather than a connect that would be refused. Google and IMAP are
 * unaffected: this is one vendor's configuration, not a broken screen.
 */
export const MicrosoftAppMissing: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_missing" },
    { provider: "imap", reason: "ready" },
  ]),
};

/**
 * A Microsoft app is registered and its secret will not open. The same badge
 * as above, and a different panel once picked: telling this operator that no
 * app exists sends them to register a second one.
 */
export const MicrosoftAppUnusable: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_unusable" },
    { provider: "imap", reason: "ready" },
  ]),
};

/**
 * A deployment that does not serve Microsoft at all: nobody's setting, and
 * said as such.
 */
export const MicrosoftUnsupported: Story = {
  render: act(state("cn.consent"), {}, undefined, [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "unsupported" },
    { provider: "imap", reason: "ready" },
  ]),
};

/** At 390px the accounts stack, and the ask's Connect stays the one primary. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: act(state("cn.consent")),
};

// A mailbox already live, as the roster reports it: the account panel, the
// posture, the import and the footer that starts it.
// A calendar row carries the write grant bookings need unless a story drops it.
const mailbox = (
  provider: string,
  backfill: Record<string, unknown>,
  scopes: string[] = provider === "gcal"
    ? ["https://www.googleapis.com/auth/calendar.events"]
    : ["read"],
) => ({
  id: provider,
  provider,
  status: "connected",
  scopes,
  account_label: "anna@example.com",
  backfill,
});

function connected(
  rows: Record<string, unknown>[],
  route: { outcome?: string; returningProvider?: string } = {},
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ channel_connection: ["read", "create"] }),
      "GET /connectors": () =>
        jsonResponse({ data: rows, providers: ALL_READY }),
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false }),
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          after_date: "2026-04-02",
          estimated_messages: 4820,
          estimated_cost_minor: 310,
          currency: "USD",
          computed_at: "2026-10-02T09:00:00Z",
        }),
    });
    return (
      <StoryProviders>
        <ConnectAct
          state={state("cn.consent")}
          dispatch={() => {}}
          persist={async () => true}
          outcome={route.outcome}
          returningProvider={route.returningProvider}
        />
      </StoryProviders>
    );
  };
}

const GOOGLE_BOTH = [
  mailbox("gmail", { state: "none" }),
  mailbox("gcal", { state: "none" }),
];

/** Google mail and calendar live: one primary, "Import 6 months and continue". */
export const ConnectedGoogle: Story = { render: connected(GOOGLE_BOTH) };

export const ConnectedGoogleDark: Story = {
  render: connected(GOOGLE_BOTH),
  globals: { theme: "dark" },
};

/** The calendar skipped or refused: its row offers the calendar alone. */
export const ConnectedMailOnly: Story = {
  render: connected([mailbox("gmail", { state: "none" })]),
};

export const ConnectedMailOnlyDark: Story = {
  render: connected([mailbox("gmail", { state: "none" })]),
  globals: { theme: "dark" },
};

const GOOGLE_READ_ONLY = [
  mailbox("gmail", { state: "none" }),
  mailbox("gcal", { state: "none" }, ["read"]),
];

/** A calendar granted for reading only: Read-only badge and Reconnect, as on
 *  Connections and Meetings, since bookings cannot land on it. */
export const ConnectedCalendarReadOnly: Story = {
  render: connected(GOOGLE_READ_ONLY),
};

export const ConnectedCalendarReadOnlyDark: Story = {
  render: connected(GOOGLE_READ_ONLY),
  globals: { theme: "dark" },
};

/** A chained press whose calendar leg was refused: the mailbox stays, the
 *  refusal is named, and the calendar row asks for it again on its own. */
export const ConnectedCalendarRefused: Story = {
  render: connected([mailbox("gmail", { state: "none" })], {
    outcome: "denied",
    returningProvider: "gcal",
  }),
};

export const ConnectedCalendarRefusedDark: Story = {
  render: connected([mailbox("gmail", { state: "none" })], {
    outcome: "denied",
    returningProvider: "gcal",
  }),
  globals: { theme: "dark" },
};

/** Back on the step with the import already running: Continue is the way on. */
export const ConnectedImporting: Story = {
  render: connected([
    mailbox("gmail", {
      state: "running",
      estimated_messages: 4820,
      counts: { messages_scanned: 1200, captured: 310 },
    }),
  ]),
};

/** Another mail host: no calendar row and no import, only Continue. */
export const ConnectedImap: Story = {
  render: connected([mailbox("imap", {})]),
};

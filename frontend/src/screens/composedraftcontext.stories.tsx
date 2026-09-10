// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../api/schema";
import {
  AccountDraftContext,
  DraftOffer,
  DraftReasons,
  openCited,
  type PendingAction,
} from "./composedraftcontext";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
// The drawer's own sheet. In the app it arrives through compose.tsx's
// side-effect import; this file takes only a TYPE from compose.tsx, so the
// module graph of a story here stops short of the styles and every frame
// would picture the offer bar and the chip run unstyled.
import "./compose.css";

// What a draft stands on, from the three sides composedraftcontext.tsx draws
// it: the pickers that say which record the message is about, the offer that
// asks a model for words, and the reasons panel that says what was read.
//
// Every fixture is the shape the compose suites already exercise
// (compose-links.test.tsx's account view, compose.test.tsx's draft) rather
// than one invented here, so a frame in the catalog shows what a real run does.

const ORG_ID = "org-1";

// The account view the grounding pickers are populated from: two contacts and
// two open deals, so each pick is a real choice rather than the only option.
// Mirrors compose-links.test.tsx's ORG_VIEW.
const ORG_VIEW = {
  organization: { id: ORG_ID, name: "Acme" },
  people: {
    data: [
      { person_id: "per-1", full_name: "Dieter Klein" },
      { person_id: "per-2", full_name: "Sara Vogel" },
    ],
  },
  deals: {
    data: [
      { deal_id: "deal-1", name: "Acme Renewal" },
      { deal_id: "deal-2", name: "Acme Expansion" },
    ],
  },
};

// The same account with nobody on it: the DRAFT's honest dead end, which the
// component says in words instead of offering a picker the rep cannot use.
const ORG_VIEW_NO_CONTACTS = {
  ...ORG_VIEW,
  people: { data: [] },
  deals: { data: [] },
};

// Both shapes a reason comes in, because the panel draws them differently and
// the difference is the point: a reason carrying evidence is pressable and
// opens the record it names, a reason that is the rep's own instruction is
// flat, because there is nothing to open.
const REASONS: readonly components["schemas"]["AccountDraftReason"][] = [
  { kind: "intent", label: "Ask for the signed order" },
  {
    kind: "recipient",
    label: "Dieter Klein leads the rollout",
    evidence_ref: {
      entity_type: "person",
      entity_id: "per-1",
      name: "Dieter Klein",
    },
  },
  {
    kind: "deal",
    label: "Renewal closes this quarter",
    evidence_ref: {
      entity_type: "deal",
      entity_id: "deal-1",
      name: "Acme Renewal",
    },
  },
];

/**
 * The pickers own no state, so the story lends them some: a reader in the
 * catalog can choose a contact and a deal and watch both selects settle, which
 * is the half of this component a static frame cannot show.
 */
function Pickers() {
  const [recipientId, setRecipientId] = useState("");
  const [dealId, setDealId] = useState("");
  return (
    <StoryProviders>
      <AccountDraftContext
        orgId={ORG_ID}
        recipientId={recipientId}
        onRecipientChange={setRecipientId}
        dealId={dealId}
        onDealChange={setDealId}
      />
    </StoryProviders>
  );
}

// Installs the stub from the story's own `render()`, which runs before any
// mount effect, so the 360 query's first call always sees it — the ordering
// the compose suites rely on too.
function pickersOver(view: unknown) {
  return () => {
    installFetchStub({
      [`GET /organizations/${ORG_ID}/360`]: () => jsonResponse(view),
    });
    return <Pickers />;
  };
}

// A control that is neither pending nor barred. `run` is inert here on purpose:
// the catalog has no model behind the button, and a story that faked a draft
// arriving would be picturing words no endpoint wrote.
const READY: PendingAction = {
  run: () => {},
  pending: false,
  disabled: false,
  error: null,
};

/** The offer bar with the intent field lent state, so it can be typed into. */
function Offer({
  draft,
  initialIntent = "",
}: Readonly<{ draft: PendingAction; initialIntent?: string }>) {
  const [intent, setIntent] = useState(initialIntent);
  return (
    <StoryProviders>
      <DraftOffer
        intent={intent}
        onIntentChange={setIntent}
        draft={draft}
        unavailable={null}
      />
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Patterns/Compose draft context",
};
export default meta;

type Story = StoryObj;

/** The account path's two choices: who the draft is to, and which deal it is about. */
export const AccountPickers: Story = {
  render: pickersOver(ORG_VIEW),
};

/**
 * An account with no contact on it. The draft has no relationship to write
 * from, so the component says so rather than offering an empty picker — the
 * rep can still type an address into To and write the mail themselves.
 */
export const NoGroundableRecipient: Story = {
  render: pickersOver(ORG_VIEW_NO_CONTACTS),
};

/** The pre-draft drawer: what the rep wants said, and the one control that asks for it. */
export const OfferReady: Story = {
  render: () => (
    <Offer draft={READY} initialIntent="Ask for the signed order" />
  ),
};

/** The model already at work — the verb goes busy and the control bars itself. */
export const OfferPending: Story = {
  render: () => (
    <Offer
      draft={{ ...READY, pending: true, disabled: true }}
      initialIntent="Ask for the signed order"
    />
  ),
};

/**
 * The draft that did not land. The failure appears without any navigation, so
 * it is announced as well as coloured: `role="alert"` carries it to a reader
 * who cannot see the line, and the ink is `--dangerText` rather than the
 * caption grey it shared with the "no model" notice — a refusal and an
 * unavailability read as the same sentence otherwise.
 */
export const OfferFailed: Story = {
  render: () => (
    <Offer
      draft={{
        ...READY,
        error: "The draft could not be written. Try again in a moment.",
      }}
      initialIntent="Ask for the signed order"
    />
  ),
};

/**
 * The same refusal in dark, where `--dangerText` is the token that has to lift
 * off the drawer's ground for the line to stay legible.
 */
export const OfferFailedDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <Offer
      draft={{
        ...READY,
        error: "The draft could not be written. Try again in a moment.",
      }}
      initialIntent="Ask for the signed order"
    />
  ),
};

/**
 * What the draft was written from, in the two shapes State D draws: the "Based
 * on" line for scanning before reading the draft, and the "Why this draft?"
 * chips for checking one input after. `openCited` is the real handler the
 * composer passes, so a chip routes exactly where the drawer sends it.
 */
export const Reasons: Story = {
  render: () => (
    <StoryProviders>
      <DraftReasons reasons={REASONS} onOpenRecord={openCited} />
    </StoryProviders>
  ),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { openCitation } from "../records/record360";
import {
  type DraftNeed,
  DraftOffer,
  DraftReasons,
  type PendingAction,
  RelatedFiling,
} from "./composedraftcontext";
// The drawer's own sheet. In the app it arrives through compose.tsx's
// side-effect import; this file takes only a TYPE from compose.tsx, so the
// module graph of a story here stops short of the styles and every frame
// would picture the offer bar and the chip run unstyled.
import "./compose.css";

// What a draft stands on, from the three sides composedraftcontext.tsx draws.
// The picker says which record the message is about. The offer asks a model
// for words, and the reasons panel says what was read.
//
// Every fixture is the shape the compose suites already exercise
// (compose-links.test.tsx's account view, compose.test.tsx's draft) rather
// than one invented here, so a frame in the catalog shows what a real run does.

const COMPANY_ID = "company-1";

// The account view the picker's deals are read from: two open deals, so each
// pick is a real choice rather than the only option.
// Mirrors compose-links.test.tsx's COMPANY_VIEW.
const COMPANY_VIEW = {
  company: { id: COMPANY_ID, name: "Acme" },
  deals: {
    data: [
      { deal_id: "deal-1", name: "Acme Renewal" },
      { deal_id: "deal-2", name: "Acme Expansion" },
    ],
  },
};

// The account's projects, as the composer passes them from the same view.
const PROJECTS = [
  {
    project_id: "pr-1",
    name: "Website relaunch",
    key: "WEB-1",
    phase: "delivering" as const,
  },
];

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
      entity_type: "contact",
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
 * The picker owns no state, so the story lends it some. A reader in the
 * catalog can choose a project, a deal or nothing and watch the face settle.
 */
function Related() {
  const [projectId, setProjectId] = useState("pr-1");
  const [dealId, setDealId] = useState("");
  return (
    <StoryProviders>
      <RelatedFiling
        companyId={COMPANY_ID}
        projects={PROJECTS}
        projectId={projectId}
        onProjectChange={setProjectId}
        dealId={dealId}
        onDealChange={setDealId}
      />
    </StoryProviders>
  );
}

// Installs the stub from the story's own `render()`, which runs before any
// mount effect, so the 360 query's first call always sees it — the ordering
// the compose suites rely on too.
function relatedOver(view: unknown) {
  return () => {
    installFetchStub({
      [`GET /companies/${COMPANY_ID}/360`]: () => jsonResponse(view),
    });
    return <Related />;
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
  replying = false,
  need = null,
}: Readonly<{
  draft: PendingAction;
  initialIntent?: string;
  replying?: boolean;
  need?: DraftNeed;
}>) {
  const [intent, setIntent] = useState(initialIntent);
  return (
    <StoryProviders>
      <DraftOffer
        replying={replying}
        intent={intent}
        onIntentChange={setIntent}
        draft={draft}
        unavailable={null}
        need={need}
      />
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Patterns/Compose mail/Draft context",
};
export default meta;

type Story = StoryObj;

/** What the message is about: one of the account's projects, deals, or nothing. */
export const RelatedTo: Story = {
  render: relatedOver(COMPANY_VIEW),
};

/** Nobody from the account in To yet, so the draft waits for one. */
export const OfferNeedsRecipient: Story = {
  render: () => (
    <Offer
      draft={{ ...READY, disabled: true }}
      initialIntent="Ask for the signed order"
      need="recipient"
    />
  ),
};

/**
 * An account with no contact on it. The draft has no relationship to write
 * from, so the offer says so. The rep can still type an address into To and
 * write the mail themselves.
 */
export const OfferNoContacts: Story = {
  render: () => <Offer draft={{ ...READY, disabled: true }} need="contacts" />,
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
 * What the draft was written from, shut under the "Why this draft" hint.
 * `openCitation` is the composer's own handler, so a chip routes as it does there.
 */
export const Reasons: Story = {
  render: () => (
    <StoryProviders>
      <DraftReasons reasons={REASONS} onOpenRecord={openCitation} />
    </StoryProviders>
  ),
};

export const ReplyOffer: Story = {
  render: () => (
    <Offer draft={READY} replying initialIntent="Confirm the delivery window" />
  ),
};
export const NewEmailNeedsPurpose: Story = {
  render: () => <Offer draft={{ ...READY, disabled: true }} />,
};

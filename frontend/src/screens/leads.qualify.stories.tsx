// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { QualifyDialog } from "./leads.qualify";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type Lead = components["schemas"]["Lead"];

// "This lead is now a contact, and maybe an opportunity."
//
// The dialog's three blocks are one act: what happens to the CONTACT, read
// from the server's own preview rather than guessed here; the deal it may open
// in the same transaction; and why — derived from what was captured, so a rep
// who qualifies a lead nobody has heard from is recorded as the human who
// decided it.
//
// The preview is the state worth showing more than once: "this creates a new
// contact" and "this folds into one you already have" are opposite outcomes
// from one press, and the third — a matched contact the reader may not see —
// must not read as either.

function lead(overrides: Partial<Lead> = {}): Lead {
  return {
    id: "l-1",
    full_name: "Jonas Petersen",
    email: "jonas@nordwind.example",
    company_name: "Nordwind Logistik",
    status: "engaged",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-24T11:00:00Z",
    ...overrides,
  };
}

const PIPELINES = {
  data: [
    {
      id: "p-1",
      name: "New business",
      is_default: true,
      stages: [
        { id: "s-1", name: "Discovery", semantic: "open", position: 1 },
        { id: "s-2", name: "Proposal", semantic: "open", position: 2 },
        { id: "s-3", name: "Won", semantic: "won", position: 3 },
      ],
    },
  ],
};

function dialog(props: {
  lead?: Partial<Lead>;
  preview?: () => Response | Promise<Response>;
}) {
  return () => {
    installFetchStub({
      "GET /leads/l-1/promote-preview":
        props.preview ?? (() => jsonResponse({ outcome: "create" })),
      "GET /pipelines": () => jsonResponse(PIPELINES),
      "GET /installation/settings": () =>
        jsonResponse({ base_currency: "EUR" }),
    });
    return (
      <StoryProviders>
        <QualifyDialog
          lead={lead(props.lead)}
          open
          onClose={() => undefined}
          onQualified={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof QualifyDialog> = {
  title: "Records/Leads/Qualify",
  component: QualifyDialog,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof QualifyDialog>;

/** An engaged lead: the deal block starts ticked, because a rep who has
 *  something to sell is the reason this lead reached this state. */
export const OpensWithADeal: Story = { render: dialog({}) };

/** A lead nobody has engaged yet: the same dialog with the deal block
 *  unticked, so qualifying does not quietly open an opportunity. */
export const ContactOnly: Story = {
  render: dialog({ lead: { status: "contacted" } }),
};

/** The preview says this email already belongs to somebody: the act is a
 *  MERGE, and it says so before the press rather than after it. */
export const WouldMergeIntoAnExistingContact: Story = {
  render: dialog({
    preview: () =>
      jsonResponse({
        outcome: "merge",
        person: {
          id: "p-9",
          full_name: "Jonas Petersen",
          version: 1,
          created_at: "2026-02-01T00:00:00Z",
          updated_at: "2026-08-01T00:00:00Z",
        },
      }),
  }),
};

/** A merge whose matched contact is outside the reader's row scope. The
 *  outcome is still stated — an omitted person read as "no match" would tell
 *  the rep a duplicate is about to be created when the opposite is true. */
export const WouldMergeIntoAContactYouCannotSee: Story = {
  render: dialog({
    preview: () => jsonResponse({ outcome: "merge", person_withheld: true }),
  }),
};

/** The preview in flight. The dialog says it is checking rather than
 *  reporting an outcome it does not have yet. */
export const CheckingForADuplicate: Story = {
  render: dialog({ preview: () => new Promise<Response>(() => undefined) }),
};

/** The preview refused. The reader is told the check could not run, which is
 *  a different thing from being told there is no duplicate. */
export const PreviewUnavailable: Story = {
  render: dialog({
    preview: () =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Internal Server Error",
          status: 500,
          detail: "The dedupe index is rebuilding.",
        },
        500,
      ),
  }),
};

/** At 390px the dialog is a full-screen sheet and its deal fields stack. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: dialog({}),
};

/** A lead stored with an empty name. The dialog names it twice — its heading,
 *  and the deal name it suggests — and both go through the one naming rule, so
 *  the suggestion reads as an address rather than seeding a deal name that
 *  starts with a blank and gets saved unnoticed. */
export const NamedByItsAddress: Story = {
  render: dialog({ lead: { full_name: "" } }),
};

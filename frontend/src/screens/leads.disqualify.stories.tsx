// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DisqualifyDialog } from "./leads.disqualify";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type Lead = components["schemas"]["Lead"];

// Closing a lead asks why, and the answer comes from the administered list in
// Settings › Data model rather than from a free-text box: the column exists to
// be reported on, and a batch of closures explained in forty different phrasings
// reports on nothing.
//
// The reason is REQUIRED, and the dialog says so on the control that is
// refused rather than beside it — a confirm that is dead with no sentence is a
// dead end for anyone who cannot see which field is empty.

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 41,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-24T11:00:00Z",
};

const REASONS = [
  { id: "r-1", label: "Bad timing", active: true, position: 1 },
  { id: "r-2", label: "No budget", active: true, position: 2 },
  { id: "r-3", label: "Competitor", active: true, position: 3 },
  // Retired: it stays on the leads that already carry it and is not offered
  // for a new closure.
  { id: "r-4", label: "Retired reason", active: false, position: 4 },
];

function dialog(
  reasons: () => Response | Promise<Response>,
  overrides: Partial<Lead> = {},
) {
  return () => {
    installFetchStub({ "GET /lead-disqualify-reasons": reasons });
    return (
      <StoryProviders>
        <DisqualifyDialog
          lead={{ ...lead, ...overrides }}
          open
          onClose={() => undefined}
          onDisqualified={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DisqualifyDialog> = {
  title: "Records/Leads/Disqualify",
  component: DisqualifyDialog,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DisqualifyDialog>;

/** The dialog as it opens: no reason picked, so the confirm is refused and
 *  says what would lift the refusal. Only the ACTIVE reasons are offered. */
export const AsksWhy: Story = {
  render: dialog(() => jsonResponse({ data: REASONS })),
};

/** The administered list is empty, which is an installation that has not set
 *  its reasons up yet. The reader is not left pressing a confirm that cannot
 *  be satisfied from an empty chooser. */
export const NoReasonsAdministered: Story = {
  render: dialog(() => jsonResponse({ data: [] })),
};

/** The reason list still loading: the dialog is drawn and the chooser is not
 *  yet answerable. */
export const LoadingReasons: Story = {
  render: dialog(() => new Promise<Response>(() => undefined)),
};

/** The reason list refused. The chooser has nothing to offer and the reader
 *  is told why rather than shown an empty menu. */
export const ReasonsUnavailable: Story = {
  render: dialog(() =>
    jsonResponse(
      {
        type: "about:blank",
        title: "Forbidden",
        status: 403,
        detail: "You may not read this installation's lead vocabulary.",
      },
      403,
    ),
  ),
};

/** At 390px the dialog is a full-screen sheet: the chooser, the note and the
 *  two verbs all reachable without a horizontal scroll. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: dialog(() => jsonResponse({ data: REASONS })),
};

/** A `full_name` that is present and EMPTY is not a name — nothing between a
 *  `CreateLead` body and the stored row refuses one. The dialog names the lead
 *  by its address rather than heading a destructive confirmation with a
 *  blank. */
export const NamedByItsAddress: Story = {
  render: dialog(() => jsonResponse({ data: REASONS }), { full_name: "" }),
};

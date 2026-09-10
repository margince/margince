// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyProfileForm } from "./companyprofiletab";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type Organization = components["schemas"]["Organization"];

// The Profile tab's own body, in the two states that differ by WRITE STANDING:
// every field editable, and the same fields with the verbs gone and one
// sentence at the top saying why. That sentence is the tab's read-only notice —
// it carries the id every refused control below points at, so the notice's
// heading IS the description rather than a second copy of it.

const ORG = "01a04298-1971-7076-8076-8064da20fdff";

const org = {
  writable: true,
  id: ORG,
  workspace_id: "01a04298-1971-7076-8076-8064da20fd01",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "01a04298-1971-7076-8076-8064da20fd02",
  industry: "Automotive",
  size_band: "51-200",
  domains: [{ domain: "brandt.example", is_primary: true, source: "manual" }],
  description: "Fleet electrification pilot, renewing in Q3.",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
} as unknown as Organization;

function frame(
  organization: Organization,
  allow: Parameters<typeof meRoute>[0],
) {
  const routes: RouteMap = {
    "GET /me": meRoute(allow),
    [`GET /organizations/${ORG}/profile-fields`]: () =>
      jsonResponse({ data: [] }),
    [`GET /organizations/${ORG}/facts`]: () => jsonResponse({ data: [] }),
  };
  installFetchStub(routes);
  return (
    <StoryProviders>
      <CompanyProfileForm org={organization} tools={null} />
    </StoryProviders>
  );
}

const WRITER = { organization: ["read", "update"] } as const;

const meta: Meta<typeof CompanyProfileForm> = {
  title: "Records/Company 360/Profile tab",
  component: CompanyProfileForm,
};
export default meta;

type Story = StoryObj<typeof CompanyProfileForm>;

/** The ordinary tab: the account's own fields, every one of them editable. */
export const Editable: Story = { render: () => frame(org, WRITER) };

/**
 * Archived, which is the more specific of the two denials and the one a reader
 * can act on. The notice is the tab's single explanation, and it is the state
 * the callout's `standing` kind exists for: it is true as the page renders and
 * has nothing to interrupt for.
 */
export const ReadOnlyBecauseArchived: Story = {
  render: () => frame({ ...org, archived_at: "2026-07-15T00:00:00Z" }, WRITER),
};

/** The same notice in dark, because tone reaches the heading's ink. */
export const ReadOnlyBecauseArchivedDark: Story = {
  globals: { theme: "dark" },
  render: () => frame({ ...org, archived_at: "2026-07-15T00:00:00Z" }, WRITER),
};

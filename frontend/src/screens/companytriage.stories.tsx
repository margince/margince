// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { CompanyTriageSection } from "./companytriage";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Why this company record exists: the mail domains the pipeline checked into
// it, each with the rung that settled it.
//
// The section is CLOSED at rest, so every story opens it — a capture of the
// summary alone would be a picture of a control rather than of the answer it
// holds. The three states below are the three the card is careful to keep
// apart: domains it can name, no domain ever checked in (an ordinary record,
// typed or imported), and a read that did not answer at all. Drawing the last
// two the same way would tell a member how their record came to exist on the
// strength of a request that failed.

const COMPANY_ID = "01a02000-0000-7000-8000-000000000001";

const meta: Meta<typeof CompanyTriageSection> = {
  title: "Records/Company rail/Capture triage",
  component: CompanyTriageSection,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyTriageSection>;

function rung(over: Record<string, unknown> = {}) {
  return {
    stage: "company_check",
    order: 40,
    subject_kind: "domain",
    status: "done",
    reason: "matched_domain",
    at: "2026-09-12T08:14:00Z",
    ...over,
  };
}

function section(body: unknown, { error = false } = {}) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      [`GET /companies/${COMPANY_ID}/capture-triage`]: () =>
        error
          ? jsonResponse({ title: "unavailable" }, 503)
          : jsonResponse(body),
    });
    return (
      <StoryProviders>
        <CompanyTriageSection companyId={COMPANY_ID} />
      </StoryProviders>
    );
  };
}

// The section is closed at rest, so every story opens it: the summary alone is
// a picture of a control rather than of the answer behind it. The disclosure is
// a `<summary>`, whose accessible name is the heading `SectionSummary` draws.
const openIt = async ({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) => {
  await userEvent.click(
    await within(canvasElement).findByText("Company origin"),
  );
};

/**
 * Two domains, settled differently. What to look at is the tone pair, and it
 * comes from the ladder's own map rather than a copy here: a settled rung is
 * `success` and one still running is `info` — work in flight is not a caution,
 * and amber told a member to act on a check that is simply not finished.
 */
export const Domains: Story = {
  render: section({
    domains: [
      { domain: "nordwind-logistik.example", rung: rung() },
      {
        domain: "nordwind.example",
        rung: rung({ status: "pending", reason: null, at: null }),
      },
    ],
  }),
  play: openIt,
};

/** A domain the check refused, and one it did not report on at all. */
export const RefusedAndUnreported: Story = {
  render: section({
    domains: [
      {
        domain: "brandt-automotive.example",
        rung: rung({ status: "failed", reason: "ambiguous_domain" }),
      },
      {
        domain: "brandt.example",
        rung: rung({ status: "not_reported", reason: "retention", at: null }),
      },
    ],
  }),
  play: openIt,
};

/**
 * No domain was ever checked in — a record somebody typed or imported. This is
 * an ORDINARY answer and says so; it is not the same claim as the one below.
 */
export const NothingCheckedIn: Story = {
  render: section({ domains: [] }),
  play: openIt,
};

/**
 * The read did not answer. The card says it knows nothing rather than drawing
 * the empty state, which would claim no domain was ever checked in.
 */
export const Unavailable: Story = {
  render: section(null, { error: true }),
  play: openIt,
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DomainReview } from "./worklist.domainreview";

type Question = components["schemas"]["DomainQuestion"];
const questions: Question[] = [
  {
    domain: "alias.example",
    deferred: false,
    reason_code: "forwarded_identity",
    asked_at: "2026-10-01T10:00:00Z",
    evidence_at: "2026-09-20T10:00:00Z",
    effective_url: "https://destination.example/",
    redirect_chain: ["https://alias.example/", "https://destination.example/"],
    domain_context: {
      contacts: [
        { id: "01a1a000-0000-7000-8000-00000000a1e7", name: "Alex Example" },
      ],
      contact_count: 3,
      hidden_senders: false,
      linked_contact_count: 2,
      sender_address: "alex@alias.example",
      origin_activity_id: "01a1a000-0000-7000-8000-0000000000ac",
      suggested_company: {
        id: "01a1a000-0000-7000-8000-0000000000c0",
        name: "Destination Company",
        basis: "redirect_target",
      },
    },
  },
  {
    domain: "unreadable.example",
    deferred: false,
    reason_code: "unevidenced",
    asked_at: "2026-10-01T10:00:00Z",
    domain_context: {
      contacts: [],
      contact_count: 1,
      hidden_senders: true,
      linked_contact_count: 2,
      site_read: "unread",
    },
  },
  {
    domain: "hosting.example",
    deferred: false,
    reason_code: "vendor_verdict",
    asked_at: "2026-10-01T10:00:00Z",
    note: "Classified as a mail or hosting vendor: sells managed hosting.",
    domain_context: {
      contacts: [],
      contact_count: 0,
      hidden_senders: false,
      linked_contact_count: 2,
      sender_address: "billing@hosting.example",
    },
  },
  {
    domain: "older.example",
    deferred: false,
    reason_code: "stale_evidence",
    asked_at: "2026-10-01T10:00:00Z",
    evidence_at: "2020-01-01T10:00:00Z",
  },
];

function review(initialRows: Question[], domain?: string) {
  return () => {
    let rows = initialRows;
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({ allow: { company: ["read", "update", "create"] } }),
        ),
      "GET /capture/domain-questions": () =>
        jsonResponse({ data: rows, page: { has_more: false } }),
      "PUT /worklist/done": () => {
        rows = rows.map((row) => ({ ...row, deferred: true }));
        return new Response(null, { status: 204 });
      },
      "POST /worklist/done/undo": () => {
        rows = rows.map((row) => ({ ...row, deferred: false }));
        return new Response(null, { status: 204 });
      },
    });
    return (
      <StoryProviders>
        <DomainReview domain={domain} onClose={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DomainReview> = {
  title: "Records/Worklist/Company question review",
  component: DomainReview,
};
export default meta;
type Story = StoryObj<typeof DomainReview>;

export const BatchEvidence: Story = { render: review(questions) };
export const BatchEvidenceDark: Story = {
  ...BatchEvidence,
  globals: { theme: "dark" },
};
export const BatchEvidencePhone: Story = {
  ...BatchEvidence,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const Empty: Story = { render: review([]) };
export const SingleUndated: Story = {
  render: review(questions.filter((row) => row.reason_code === "unevidenced")),
  play: async () => {
    const body = within(document.body);
    await waitFor(() =>
      expect(body.getByRole("button", { name: "Not now" })).toBeVisible(),
    );
    await userEvent.click(body.getByRole("button", { name: "Not now" }));
    await waitFor(() =>
      expect(
        body.getByRole("button", { name: "Return to Review" }),
      ).toBeVisible(),
    );
    await expect(
      body.getByText("Which company does unreadable.example belong to?"),
    ).toBeVisible();
  },
};

export const ForwardedIdentityNeedsAMatch: Story = {
  render: review(
    questions.filter((row) => row.reason_code === "forwarded_identity"),
  ),
  play: async () => {
    const body = within(document.body);
    await waitFor(() =>
      expect(
        body.getByText(
          "Which company do Alex Example and 2 other contacts work for?",
        ),
      ).toBeVisible(),
    );
    await expect(
      body.queryByRole("button", { name: "Create company" }),
    ).not.toBeInTheDocument();
    await expect(
      body.getByRole("button", { name: "Match company" }),
    ).toBeDisabled();
  },
};

/** Opened from a row: that one question, with its website details closed. */
export const FocusedFromARow: Story = {
  render: review(
    questions.filter((row) => row.domain === "alias.example"),
    "alias.example",
  ),
};
export const FocusedFromARowDark: Story = {
  ...FocusedFromARow,
  globals: { theme: "dark" },
};

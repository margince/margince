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
        { id: "00000000-0000-4000-8000-0000000000a1", name: "Alex Example" },
      ],
      contact_count: 1,
      hidden_senders: false,
      site_unread: false,
    },
  },
  {
    domain: "unreadable.example",
    deferred: false,
    reason_code: "unevidenced",
    asked_at: "2026-10-01T10:00:00Z",
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
  title: "Records/Worklist/Domain maintenance review",
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
    await expect(body.getByText("unreadable.example")).toBeVisible();
  },
};

export const ForwardedIdentityNeedsAMatch: Story = {
  render: review(
    questions.filter((row) => row.reason_code === "forwarded_identity"),
  ),
  play: async () => {
    const body = within(document.body);
    await waitFor(() => expect(body.getByText("alias.example")).toBeVisible());
    await expect(
      body.queryByRole("button", { name: "Create company" }),
    ).not.toBeInTheDocument();
    await expect(
      body.getByRole("button", { name: "Match company" }),
    ).toBeDisabled();
  },
};

/** Opened from a row: only that question, its website details folded away. */
export const FocusedOnOneQuestion: Story = {
  render: review(
    questions.filter((row) => row.domain === "alias.example"),
    "alias.example",
  ),
  play: async () => {
    const body = within(document.body);
    await waitFor(() =>
      expect(body.getByText("Company for alias.example")).toBeVisible(),
    );
    await expect(
      body.queryByText("unreadable.example"),
    ).not.toBeInTheDocument();
  },
};
export const FocusedOnOneQuestionDark: Story = {
  ...FocusedOnOneQuestion,
  globals: { theme: "dark" },
};

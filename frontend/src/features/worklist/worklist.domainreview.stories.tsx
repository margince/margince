// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
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

function review(initialRows: Question[]) {
  return () => {
    let rows = initialRows;
    installFetchStub({
      "GET /capture/domain-questions": () =>
        jsonResponse({ data: rows, page: { has_more: false } }),
      "PUT /worklist/done": () => {
        rows = rows.map((row) => ({ ...row, deferred: true }));
        return jsonResponse(null, 204);
      },
      "POST /worklist/done/undo": () => {
        rows = rows.map((row) => ({ ...row, deferred: false }));
        return jsonResponse(null, 204);
      },
    });
    return (
      <StoryProviders>
        <DomainReview onClose={() => {}} />
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
export const Empty: Story = { render: review([]) };
export const SingleUndated: Story = {
  render: review(questions.filter((row) => row.reason_code === "unevidenced")),
  play: async () => {
    const body = within(document.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Set aside" }),
    );
    await expect(
      body.findByRole("button", { name: "Return to Review" }),
    ).resolves.toBeVisible();
    await expect(body.getByText("unreadable.example")).toBeVisible();
  },
};

export const ForwardedIdentityNeedsAMatch: Story = {
  render: review(
    questions.filter((row) => row.reason_code === "forwarded_identity"),
  ),
  play: async () => {
    const body = within(document.body);
    await expect(body.findByText("alias.example")).resolves.toBeVisible();
    await expect(
      body.queryByRole("button", { name: "Create company" }),
    ).not.toBeInTheDocument();
    await expect(
      body.getByRole("button", { name: "Match company" }),
    ).toBeDisabled();
  },
};

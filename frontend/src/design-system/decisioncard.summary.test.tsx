// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PLAIN_SUMMARY_WORDS } from "./approvalsummary.testkit";
import {
  type DecisionApproval,
  DecisionCard,
  type DecisionCardLabels,
} from "./decisioncard";

afterEach(cleanup);

const NOW = Date.parse("2026-08-24T09:00:00.000Z");

const LABELS: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  expired: "This ran out of time.",
  draftSubject: "Subject",
  draftBody: "Message",
  summaryWords: PLAIN_SUMMARY_WORDS,
  noContent: "This proposal carries nothing to read.",
  loading: "Reading the proposal",
};

const staged: DecisionApproval = {
  id: "0198c4f1-2b6a-7c3d-9e0f-11223344aabb",
  kind: "update_record",
  status: "pending",
  proposed_by: "agent:mailroom",
  created_at: "2026-08-24T07:41:00.000Z",
  expires_at: "2026-08-24T18:00:00.000Z",
  proposed_change: {},
  summary: "Update record deal: name=Acme, Inc. renewal",
  summary_parts: {
    sentence: "Update record deal",
    fields: [{ field: "name", value: "Acme, Inc. renewal" }],
    more_fields: 0,
  },
};

describe("DecisionCard — a summary staged with its parts", () => {
  it("heads with the sentence and lists each proposed value whole", () => {
    render(
      <DecisionCard
        approval={staged}
        now={NOW}
        labels={LABELS}
        onAccept={() => undefined}
        onReject={() => undefined}
      />,
    );
    expect(
      screen.getByRole("article", { name: "Update record deal" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Acme, Inc. renewal")).toBeInTheDocument();
    expect(screen.queryByText(/name=/)).toBeNull();
  });

  it("shows an old row's whole summary as its headline", () => {
    render(
      <DecisionCard
        approval={{ ...staged, summary_parts: undefined }}
        now={NOW}
        labels={LABELS}
        onAccept={() => undefined}
        onReject={() => undefined}
      />,
    );
    expect(
      screen.getByRole("article", {
        name: "Update record deal: name=Acme, Inc. renewal",
      }),
    ).toBeInTheDocument();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  type DecisionApproval,
  DecisionCard,
  type DecisionCardLabels,
} from "./decisioncard";
import type { DecisionDisplay } from "./decisioncard.payload";

afterEach(cleanup);

// What a card declines to draw. No sentence twice, no provenance prose dressed
// as a draft, and no wire enum where a source goes.

const NOW = Date.parse("2026-10-19T09:00:00.000Z");

const LABELS: DecisionCardLabels = {
  accept: "Freigeben",
  edit: "Bearbeiten",
  reject: "Ablehnen",
  expired: "Abgelaufen.",
  draftSubject: "Betreff",
  draftBody: "Nachricht",
  summaryWords: {
    field: (name) => name,
    more: (count) => `${count} more not shown`,
  },
  noContent: "Nichts zu lesen.",
  loading: "Wird gelesen",
};

const SUMMARY = "Send the signed form by Friday";
const BODY =
  'Promiser: Lena Fischer. Commitment in the email "Contract", sent 2026-10-01.';

const commitment: DecisionApproval = {
  id: "0198c4f1-2b6a-7c3d-9e0f-11223344aacc",
  kind: "commitment_task",
  status: "pending",
  proposed_by: "agent:signal-scan",
  created_at: "2026-10-19T07:00:00.000Z",
  summary: SUMMARY,
  target_label: "Contract",
  proposed_change: {
    summary: SUMMARY,
    party: "Lena Fischer",
    body: BODY,
  },
  evidence: [
    {
      evidence_snippet: "I will send the signed form by Friday",
      source_type: "activity",
      source_id: "0198c4f1-2b6a-7c3d-9e0f-11223344aadd",
    },
  ],
};

const display: readonly DecisionDisplay[] = [
  { field: "summary", label: "Schritt", value: SUMMARY, lead: true },
  { field: "party", label: "Zugesagt von", value: "Dein Team" },
  { field: "body", label: "Originaltext", value: BODY },
];

function drawn(approval: DecisionApproval, layout: "deck" | "row") {
  render(
    <LocaleProvider initial="de">
      <DecisionCard
        approval={approval}
        layout={layout}
        now={NOW}
        labels={LABELS}
        display={display}
        onAccept={() => undefined}
      />
    </LocaleProvider>,
  );
}

describe("DecisionCard — what it leaves out", () => {
  it.each(["deck", "row"] as const)("shows the summary once (%s)", (layout) => {
    drawn(commitment, layout);
    expect(screen.getAllByText(SUMMARY)).toHaveLength(1);
    expect(screen.getByText("Dein Team")).toBeInTheDocument();
  });

  it("still leads with a payload sentence that differs from the summary", () => {
    drawn(
      { ...commitment, summary: "A commitment from the Contract thread" },
      "deck",
    );
    expect(screen.getByText(SUMMARY)).toBeInTheDocument();
  });

  // The summary the server stores is cleaned and bounded; the payload's copy
  // is raw. Both still read as one sentence.
  it.each([
    ["extra whitespace", `  Send the signed\n form   by Friday `],
    ["a bidi control", "Send the signed form‮ by Friday"],
  ])("treats a lead with %s as the same sentence", (_, lead) => {
    const leadDisplay = [
      { field: "summary", label: "Schritt", value: lead, lead: true },
    ];
    render(
      <DecisionCard
        approval={{ ...commitment, proposed_change: { summary: lead } }}
        now={NOW}
        labels={LABELS}
        display={leadDisplay}
      />,
    );
    expect(screen.getAllByText(SUMMARY)).toHaveLength(1);
    expect(screen.queryByText(lead)).toBeNull();
  });

  it("matches a lead to the bounded summary cut from it", () => {
    const long = `${SUMMARY} and countersign the annex we discussed on the call`;
    render(
      <DecisionCard
        approval={{ ...commitment, summary: `${SUMMARY} and…` }}
        now={NOW}
        labels={LABELS}
        display={[
          { field: "summary", label: "Schritt", value: long, lead: true },
        ]}
      />,
    );
    expect(screen.queryByText(long)).toBeNull();
  });

  it("draws a declared body as its labelled fact, not as a draft", () => {
    drawn(commitment, "deck");
    expect(screen.getAllByText(BODY)).toHaveLength(1);
    expect(screen.getByText("Originaltext")).toBeInTheDocument();
    expect(screen.queryByText("Nachricht")).toBeNull();
  });

  it("names the evidence source in the reader's language", () => {
    drawn(commitment, "deck");
    expect(screen.getByText(/Aktivität/)).toBeInTheDocument();
    expect(screen.queryByText(/· activity/)).toBeNull();
  });
});

it.each(["https://source.example.test/about", "javascript:alert(1)"])(
  "only opens safe source links: %s",
  (href) => {
    render(
      <LocaleProvider initial="de">
        <DecisionCard
          approval={commitment}
          layout="deck"
          now={NOW}
          labels={LABELS}
          display={[
            { field: "source", label: "Quelle", value: "Source page", href },
          ]}
          onAccept={() => undefined}
        />
      </LocaleProvider>,
    );
    if (href.startsWith("https:")) {
      expect(screen.getByRole("link", { name: "Source page" })).toHaveAttribute(
        "href",
        href,
      );
      expect(screen.getByRole("link", { name: "Source page" })).toHaveAttribute(
        "rel",
        "noopener noreferrer",
      );
    } else {
      expect(
        screen.queryByRole("link", { name: "Source page" }),
      ).not.toBeInTheDocument();
      expect(screen.getByText("Source page")).toBeInTheDocument();
    }
  },
);

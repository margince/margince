// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { render } from "./compose.testkit";
import type { InlineReview } from "./composeinlinereview";
import { SendAnywayRows } from "./composesendanyway";
import { DirectSendModal } from "./directsendmodal";

// The law a refusal or finding rests on is shown linked to its official text,
// and only when the server sent one. Without it every surface keeps the facts.

type CommunicationReview = components["schemas"]["CommunicationReview"];
type RefusedRecipient = CommunicationReview["refusals"][number];

const UWG = {
  instrument: "UWG",
  provision: "§ 7(2)",
  source_url: "https://www.gesetze-im-internet.de/uwg_2004/__7.html",
};
const DECREE = {
  instrument: "Decree 91/2020/ND-CP",
  provision: "Art. 8",
  source_url: "https://example.org/decree-91",
};

const objected = (extra: Partial<RefusedRecipient> = {}): RefusedRecipient => ({
  address: "anna@example.de",
  reason_code: "marketing_objection",
  ...extra,
});

const reviewOf = (refusals: RefusedRecipient[]): CommunicationReview => ({
  id: "r-1",
  state: "needs_repair",
  kind: "single",
  reason_code: refusals[0]?.reason_code ?? "",
  refusals,
  warning: { version: "override-v2", text: "The send check still blocks it." },
});

const shownReview = (review: CommunicationReview): InlineReview => ({
  refusal: null,
  shown: true,
  review,
  unreviewed: false,
  loading: false,
  loadFailed: false,
  retryLoad: () => {},
  canSendAnyway: false,
  confirm: () => {},
  pending: false,
  sending: false,
  changed: false,
  failed: false,
});

function renderRows(refusals: RefusedRecipient[]) {
  return render(
    <SendAnywayRows
      state={shownReview(reviewOf(refusals))}
      explanation=""
      onExplanationChange={() => {}}
    />,
  );
}

// A link to the law opens a new tab that cannot reach back into this one.
function expectLawLink(name: string, href: string) {
  const link = screen.getByRole("link", { name });
  expect(link).toHaveAttribute("href", href);
  expect(link).toHaveAttribute("target", "_blank");
  expect(link).toHaveAttribute("rel", "noopener noreferrer");
}

afterEach(cleanup);

describe("the inline send review", () => {
  it("leads with the law the refusal rests on, linked, and keeps the fact", () => {
    renderRows([objected({ provision: UWG })]);

    const title = document.querySelector(".callout-title");
    expect(title).toHaveTextContent(
      "Attention: sending to anna@example.de may break UWG § 7(2)",
    );
    expectLawLink("UWG § 7(2)", UWG.source_url);
    expect(
      screen.getByText("anna@example.de objected to marketing."),
    ).toBeInTheDocument();
  });

  it("keeps the factual sentence and links nothing without a provision", () => {
    renderRows([objected()]);

    expect(document.querySelector(".callout-title")).toHaveTextContent(
      "anna@example.de objected to marketing.",
    );
    expect(screen.queryByText(/Attention:/)).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("names each recipient's own law in the findings behind the count", () => {
    renderRows([
      objected({ provision: UWG }),
      {
        address: "binh@example.vn",
        reason_code: "footer_incomplete",
        footer_findings: [
          {
            kind: "advertiser_contact",
            language: "",
            missing_means: [],
            provision: DECREE,
          },
        ],
      },
    ]);

    const binh = screen.getByText(/binh@example\.vn/, { selector: "li" });
    expect(
      within(binh).getByRole("link", { name: "Decree 91/2020/ND-CP Art. 8" }),
    ).toHaveAttribute("href", DECREE.source_url);
    expect(within(binh).queryByRole("link", { name: /UWG/ })).toBeNull();
  });
});

describe("the direct-send dialog", () => {
  it("cites the law on the refused row and its footer finding", () => {
    render(
      <DirectSendModal
        open
        onClose={() => {}}
        review={reviewOf([
          objected({
            provision: UWG,
            footer_findings: [
              {
                kind: "objection_route",
                language: "",
                missing_means: [],
                provision: DECREE,
              },
            ],
          }),
        ])}
      />,
    );

    expect(
      screen.getByText(/Attention: sending to anna@example\.de may break/),
    ).toBeInTheDocument();
    expectLawLink("UWG § 7(2)", UWG.source_url);
    expectLawLink("Decree 91/2020/ND-CP Art. 8", DECREE.source_url);
  });

  it("shows the bare address when no law is cited", () => {
    render(
      <DirectSendModal
        open
        onClose={() => {}}
        review={reviewOf([objected()])}
      />,
    );

    expect(screen.getByText("anna@example.de")).toBeInTheDocument();
    expect(screen.queryByText(/Attention:/)).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { isTechnicalFact, TechnicalProfileCard } from "./companytechnical";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const ORG = "019ff000-0000-7000-8000-0000000000a1";

function fact(field: string, valueKey: string, value: string) {
  return {
    id: `fact-${field}-${valueKey}`,
    category: "signal",
    field,
    value,
    value_key: valueKey,
    source: "technical_lookup",
    captured_by: "agent:technical-lookup",
    evidence_snippet: "beispiel-de.mail.protection.outlook.com",
    source_url: "dns:beispiel.de",
    confidence: 0.9,
    retrieved_at: "2026-08-27T07:12:00Z",
    updated_at: "2026-08-27T07:12:00Z",
  };
}

const CAN_ENRICH = meRoute({ organization: ["read", "update"] });

function laneState(outcome = "applied") {
  return {
    organization_id: ORG,
    lanes: [
      {
        lane: "certlog",
        outcome,
        attempts: outcome === "failed" ? 3 : 0,
        last_success_at: "2026-08-20T07:00:00Z",
        next_attempt_at: "2026-09-03T07:12:00Z",
      },
    ],
  };
}

function renderCard() {
  return render(
    <StoryProviders locale="de">
      <TechnicalProfileCard orgId={ORG} />
    </StoryProviders>,
  );
}

describe("the technical profile card", () => {
  // Rendered per test rather than per file, so a card left mounted by the
  // previous case cannot answer this one's queries.
  afterEach(cleanup);

  beforeEach(() => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({
          data: [
            fact("mail_provider", "microsoft365", "Microsoft 365"),
            fact("operated_service", "webshop", "Webshop"),
          ],
        }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(laneState()),
    });
  });

  it("shows what the company runs, grouped by what a reader asks about", async () => {
    renderCard();
    expect(await screen.findByText("Microsoft 365")).toBeTruthy();
    expect(screen.getByText("Webshop")).toBeTruthy();
    expect(screen.getByText("Mail")).toBeTruthy();
    expect(screen.getByText("Dienste")).toBeTruthy();
  });

  it("asks for a lookup when the reader presses the button", async () => {
    const user = userEvent.setup();
    let asked = 0;
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () => jsonResponse({ data: [] }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse({ title: "not found" }, 404),
      [`POST /organizations/${ORG}/technical-enrich`]: () => {
        asked += 1;
        return jsonResponse({ organization_id: ORG, status: "queued" }, 202);
      },
    });
    renderCard();

    await user.click(
      await screen.findByRole("button", { name: "Nachschauen" }),
    );

    expect(await screen.findByText(/Abfrage läuft/)).toBeTruthy();
    expect(asked).toBe(1);
  });

  // An empty card with no sentence reads as "this company runs nothing", which
  // is a different and false claim from "nobody has looked".
  it("says plainly when nothing has been looked up", async () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () => jsonResponse({ data: [] }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse({ title: "not found" }, 404),
    });
    renderCard();
    expect(await screen.findByText(/Noch nichts nachgeschaut/)).toBeTruthy();
  });

  // The notice is what tells a reader that an absent service means "not checked
  // today" rather than "they have none".
  it("names a source that did not answer", async () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({
          data: [fact("mail_provider", "microsoft365", "Microsoft 365")],
        }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(laneState("failed")),
    });
    renderCard();
    expect(await screen.findByText(/hat nicht geantwortet/)).toBeTruthy();
  });

  it("offers no lookup to a reader who may not write the record", async () => {
    installFetchStub({
      "GET /me": meRoute({ organization: ["read"] }, { seat: "read" }),
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({
          data: [fact("mail_provider", "microsoft365", "Microsoft 365")],
        }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(laneState()),
    });
    renderCard();
    expect(await screen.findByText("Microsoft 365")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Nachschauen" })).toBeNull();
  });
});

// The partition decides which card owns a row. Getting it wrong renders every
// technical fact twice on one tab, or drops a corrected one from both.
describe("which facts belong to the technical profile", () => {
  it("claims every technical field", () => {
    for (const field of [
      "mail_provider",
      "email_security",
      "hosting_provider",
      "operated_service",
      "technology",
    ]) {
      expect(
        isTechnicalFact({ category: "signal", field } as never),
        `${field} belongs on the technical card`,
      ).toBe(true);
    }
  });

  it("leaves the company's own claims to the evidence card", () => {
    expect(
      isTechnicalFact({ category: "signal", field: "certification" } as never),
    ).toBe(false);
    expect(
      isTechnicalFact({ category: "company", field: "phone" } as never),
    ).toBe(false);
  });

  // A person correcting a machine-read value rewrites the row's source to
  // `human`. Partitioning by source would drop exactly the rows somebody cared
  // enough to fix.
  it("keeps a corrected row on the technical card", () => {
    expect(
      isTechnicalFact({
        category: "signal",
        field: "mail_provider",
        source: "human",
      } as never),
    ).toBe(true);
  });
});

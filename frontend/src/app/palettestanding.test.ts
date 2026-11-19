// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { type Translator, translate } from "../i18n";
import { describeHit } from "./palettestanding";

type SearchResult = components["schemas"]["SearchResult"];

const t: Translator = (key, params) => translate("en", key, params);
const reading = {
  t,
  locale: "en" as const,
  zone: "UTC",
  now: new Date("2026-11-17T12:00:00Z"),
};

function hit(type: string, standing?: SearchResult["standing"]): SearchResult {
  return { type, id: "00000000-0000-4000-8000-000000000001", standing };
}

describe("describeHit", () => {
  it("gives an open deal its amount, its close date and its stage", () => {
    expect(
      describeHit(
        hit("deal", {
          state: "open",
          stage: "Negotiation",
          amount_minor: 42_000_000,
          currency: "EUR",
          due_on: "2026-12-12",
        }),
        undefined,
        reading,
      ),
    ).toEqual({
      subtitle: "€420,000 · closes 12 Dec",
      status: { kind: "neutral", label: "Negotiation" },
    });
  });

  it("names a won deal won rather than by its stage, and drops the close date", () => {
    expect(
      describeHit(
        hit("deal", {
          state: "won",
          stage: "Closed won",
          amount_minor: 6_400_000,
          currency: "EUR",
          due_on: "2026-10-02",
        }),
        undefined,
        reading,
      ),
    ).toEqual({
      subtitle: "€64,000",
      status: { kind: "done", label: "Won" },
    });
  });

  it("prints no amount the reader may not see", () => {
    const description = describeHit(
      hit("deal", {
        state: "open",
        stage: "Proposal",
        masked_fields: ["amount_minor", "currency"],
      }),
      undefined,
      reading,
    );
    expect(description.subtitle).toBeUndefined();
    expect(description.status?.label).toBe("Proposal");
  });

  it("marks a new lead as waiting on someone", () => {
    expect(
      describeHit(hit("lead", { state: "new" }), "Acme Robotics AG", reading),
    ).toEqual({
      subtitle: "Acme Robotics AG",
      status: { kind: "attention", label: "New" },
    });
  });

  it("gives a project its phase and appends its due date to the key", () => {
    expect(
      describeHit(
        hit("project", { state: "delivering", due_on: "2027-01-30" }),
        "ACME-12",
        reading,
      ),
    ).toEqual({
      subtitle: "ACME-12 · due 30 Jan 2027",
      status: { kind: "neutral", label: "Delivering" },
    });
  });

  it("gives a contact its job title and when it was last in touch", () => {
    expect(
      describeHit(
        hit("contact", {
          job_title: "Head of Procurement",
          last_activity_at: "2026-11-14T12:00:00Z",
        }),
        undefined,
        reading,
      ),
    ).toEqual({
      subtitle: "Head of Procurement",
      aside: "Last contact 3 days ago",
    });
  });

  it("keeps the employer a contact was found through over its title", () => {
    expect(
      describeHit(
        hit("contact", { job_title: "Head of Procurement" }),
        "Works at Acme GmbH",
        reading,
      ).subtitle,
    ).toBe("Works at Acme GmbH");
  });

  it("falls back to the plain second line for a hit with no standing", () => {
    expect(describeHit(hit("product"), "KAR-9910", reading)).toEqual({
      subtitle: "KAR-9910",
    });
  });
});

/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { ExtensionIngestHealthCard } from "./extingesthealth";
// The settings harness, rather than a fourth copy of the same twenty lines:
// this card is a settings card, and `render` there already provides the two
// providers every one of them needs.
import { jsonResponse, render } from "./settings.testkit";

// Every request the card could make, recorded: the ABSENCE of one is what the
// withheld case is about (the harness shape the sibling health cards use).
type Sent = { key: string };

function stubRoutes(overrides: Record<string, () => Response> = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push({ key });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /admin/extension-ingest-health")
        return jsonResponse(HEALTH);
      // The endpoint gates on `job_health:read`, so the default principal here
      // holds it; the withheld case overrides with one who does not.
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

const HEALTH = {
  generated_at: "2026-09-17T09:30:00Z",
  window_days: 7,
  units: [
    {
      unit: "openchannel",
      refused: 48,
      last_refused_at: "2026-09-17T04:00:00Z",
      refusals: [
        {
          refusal: "participants",
          refused: 45,
          last_refused_at: "2026-09-17T04:00:00Z",
        },
        { refusal: "key", refused: 3, last_refused_at: "2026-09-16T11:00:00Z" },
      ],
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("names the unit and which check refused, so an operator knows what to fix", async () => {
  stubRoutes();

  render(<ExtensionIngestHealthCard />);

  expect(await screen.findByText("openchannel")).toBeInTheDocument();
  // The class in the reader's words rather than the wire's token: the point of
  // the card is to say which mapping to look at.
  expect(await screen.findByText(/45 on participants/)).toBeInTheDocument();
  expect(await screen.findByText(/3 on record key/)).toBeInTheDocument();
  expect(await screen.findByText(/48 records refused/)).toBeInTheDocument();
});

it("says nothing was refused rather than showing an empty card", async () => {
  stubRoutes({
    "GET /admin/extension-ingest-health": () =>
      jsonResponse({ ...HEALTH, units: [] }),
  });

  render(<ExtensionIngestHealthCard />);

  // The clean state is a FINDING — every record was representable — and it
  // names the window, because a reassurance with no span is one the reader has
  // to guess the meaning of.
  expect(
    await screen.findByText(/No records refused in the last 7 days/),
  ).toBeInTheDocument();
});

it("reports a malformed payload rather than drawing it as a clean installation", async () => {
  stubRoutes({
    "GET /admin/extension-ingest-health": () =>
      jsonResponse({ generated_at: "2026-09-17T09:30:00Z" }),
  });

  render(<ExtensionIngestHealthCard />);

  // `?? []` here would claim every record was representable, which is the one
  // reassurance an operator opens this card to trust. The card says the report
  // could not be read instead.
  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(
    screen.queryByText(/No records refused in the last/),
  ).not.toBeInTheDocument();
});

it("withholds the card from a seat without the grant, and asks the server nothing", async () => {
  const sent = stubRoutes({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
  });

  render(<ExtensionIngestHealthCard />);

  expect(
    await screen.findByText(/requires a permission your role does not have/),
  ).toBeInTheDocument();
  // A refusal the reader cannot act on has no business becoming this card's
  // error state, so the call is never issued.
  expect(sent.some((s) => s.key === "GET /admin/extension-ingest-health")).toBe(
    false,
  );
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
import type { WorklistItem } from "./worklist.queries";
import { TagSuggestionDecision } from "./worklist.tagsuggestion";

// A tag suggestion under its Worklist row: what it cites, and the two answers.

const SUGGESTION = "01a0e9f1-0000-7000-8000-0000000000d1";
const CONTACT = "01a0e9f1-0000-7000-8000-0000000000c1";
const COMPANY = "01a0e9f1-0000-7000-8000-0000000000a1";

const row: WorklistItem = {
  id: SUGGESTION,
  source: "tag_suggestion",
  category: "decisions",
  level: 6,
  consequence: "data_drifts",
  because: [],
  actions: ["decide", "dismiss", "open"],
  subject: { type: "contact", id: CONTACT, label: "Anna Weber" },
};

const suggestion = {
  id: SUGGESTION,
  state: "open",
  tag: { tag_id: "t-1", name: "Product X" },
  entity_type: "contact",
  entity_id: CONTACT,
  entity_name: "Anna Weber",
  created_at: "2026-10-08T09:00:00Z",
  evidence: [
    {
      activity_id: "01a0e9f1-0000-7000-8000-0000000000e1",
      kind: "email",
      subject: "Pricing for Product X",
      occurred_at: "2026-10-07T14:00:00Z",
    },
  ],
};

function mount(routes: RouteMap = {}) {
  const posted: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ company: ["update"], contact: ["update"] } as never),
    [`GET /tag-suggestions/${SUGGESTION}`]: () => jsonResponse(suggestion),
    [`POST /tag-suggestions/${SUGGESTION}/accept`]: () => {
      posted.push("accept");
      return jsonResponse({ ...suggestion, state: "accepted" });
    },
    [`POST /tag-suggestions/${SUGGESTION}/dismiss`]: () => {
      posted.push("dismiss");
      return jsonResponse({ ...suggestion, state: "dismissed" });
    },
    [`GET /contacts/${CONTACT}`]: () =>
      jsonResponse({
        id: CONTACT,
        full_name: "Anna Weber",
        employer: { company_id: COMPANY, company_name: "Demo GmbH" },
      }),
    [`GET /companies/${COMPANY}`]: () =>
      jsonResponse({ id: COMPANY, name: "Demo GmbH", writable: true }),
    [`GET /records/company/${COMPANY}/tags`]: () =>
      jsonResponse({ data: [], withheld: false }),
    ...routes,
  });
  render(
    <StoryProviders>
      <ToastProvider>
        <TagSuggestionDecision item={row} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
  return posted;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a tag suggestion on the Worklist", () => {
  it("cites the mail that raised it", async () => {
    mount();
    expect(
      await screen.findByText(/Email · Pricing for Product X/),
    ).toBeInTheDocument();
  });

  it("applies the tag on Accept and then offers it to the contact's company", async () => {
    const user = userEvent.setup();
    const posted = mount();

    await user.click(
      await screen.findByRole("button", { name: en["tagSuggestion.accept"] }),
    );

    expect(
      await screen.findByRole("button", { name: "Tag Demo GmbH" }),
    ).toBeInTheDocument();
    expect(posted).toEqual(["accept"]);
  });

  it("records a dismissal and applies nothing", async () => {
    const user = userEvent.setup();
    const posted = mount();

    await user.click(
      await screen.findByRole("button", { name: en["tagSuggestion.dismiss"] }),
    );

    expect(
      await screen.findByText(en["tagSuggestion.dismissed"]),
    ).toBeInTheDocument();
    expect(posted).toEqual(["dismiss"]);
  });

  it("says the suggestion was decided when it is no longer there to read", async () => {
    mount({
      [`GET /tag-suggestions/${SUGGESTION}`]: () =>
        jsonResponse({ title: "Not found" }, 404),
    });
    expect(
      await screen.findByText(en["tagSuggestion.decided"]),
    ).toBeInTheDocument();
  });
});

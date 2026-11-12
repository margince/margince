// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LinkSuggestionsPanel } from "./worklist.linksuggestions";

// The review queue of suggested links: a reader accepts or rejects each one.

const SUGGESTION = "01a0e9f1-0000-7000-8000-0000000000d1";

const suggestion = {
  id: SUGGESTION,
  activity_id: "01a0e9f1-0000-7000-8000-0000000000e1",
  activity_subject: "Which project is this?",
  entity_type: "deal",
  entity_id: "01a0e9f1-0000-7000-8000-0000000000f1",
  entity_name: "Renewal",
  unit: "finance-demo",
  reason: "The subject names this deal.",
  score: 0.8,
  status: "pending",
  created_at: "2026-10-08T09:00:00Z",
};

function mount(queue: unknown[]) {
  const posted: string[] = [];
  let reads = 0;
  installFetchStub({
    "GET /me": meRoute({ activity: ["update"] } as never),
    "GET /link-suggestions": () => {
      reads += 1;
      return jsonResponse({ data: queue });
    },
    [`POST /link-suggestions/${SUGGESTION}/accept`]: () => {
      posted.push("accept");
      return jsonResponse({ ...suggestion, status: "accepted" });
    },
    [`POST /link-suggestions/${SUGGESTION}/reject`]: () => {
      posted.push("reject");
      return jsonResponse({ ...suggestion, status: "rejected" });
    },
  });
  const { container } = render(
    <StoryProviders>
      <LinkSuggestionsPanel />
    </StoryProviders>,
  );
  return { posted, reads: () => reads, container };
}

afterEach(cleanup);

describe("LinkSuggestionsPanel", () => {
  it("shows what an extension suggested and why", async () => {
    mount([suggestion]);
    expect(
      await screen.findByText(/Which project is this\? to Renewal \(deal\)/),
    ).toBeInTheDocument();
    expect(
      screen.getByText("finance-demo: The subject names this deal."),
    ).toBeInTheDocument();
  });

  it("files the message only when the reader accepts", async () => {
    const { posted } = mount([suggestion]);
    const accept = await screen.findByRole("button", {
      name: en["linkSuggestions.accept"],
    });
    expect(posted).toEqual([]);
    await userEvent.click(accept);
    expect(posted).toEqual(["accept"]);
  });

  it("rejects without filing", async () => {
    const { posted } = mount([suggestion]);
    await userEvent.click(
      await screen.findByRole("button", { name: en["linkSuggestions.reject"] }),
    );
    expect(posted).toEqual(["reject"]);
  });

  it("draws nothing for an empty queue", async () => {
    const { reads, container } = mount([]);
    await waitFor(() => expect(reads()).toBe(1));
    expect(container).toBeEmptyDOMElement();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithQueryClient } from "../testing/queryrender";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../testing/story-utils";
import { ConversationSuggestionsCard } from "./conversationsuggestions";

const bob = {
  address: "bob@buyer.test",
  display_name: "Bob Buyer",
  activity_id: "activity-1",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function setup(
  type: "contact" | "lead",
  answer = () => jsonResponse({ id: "contact-new" }, 201),
  initial = [bob],
  knownAfterWrite = false,
) {
  let rows = initial;
  let reads = 0;
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read", "create"],
      lead: ["read"],
      activity: ["read"],
    }),
    [`GET /${type}s/anchor/conversation-suggestions`]: () => {
      reads++;
      return jsonResponse({ data: rows, scan_limited: false });
    },
    "POST /contacts": (body) => {
      posted.push(body);
      const response = answer();
      if (response.ok || knownAfterWrite) rows = [];
      return response;
    },
  });
  const view = renderWithQueryClient(
    <ConversationSuggestionsCard record={{ type, id: "anchor" }} />,
  );
  return { posted, reads: () => reads, view };
}

describe("recipients copied on inbound correspondence", () => {
  it.each(["contact", "lead"] as const)(
    "creates through the normal contact writer on the %s page and removes the row",
    async (type) => {
      const { posted } = setup(type);
      fireEvent.click(
        await screen.findByRole("button", { name: "Add as contact" }),
      );
      await waitFor(() =>
        expect(posted).toEqual([
          {
            full_name: "Bob Buyer",
            emails: [
              {
                email: "bob@buyer.test",
                email_type: "work",
                is_primary: true,
                position: 0,
              },
            ],
            source: "manual",
          },
        ]),
      );
      await waitFor(() =>
        expect(screen.queryByText("bob@buyer.test")).not.toBeInTheDocument(),
      );
    },
  );

  it("keeps a concurrently created live lead and offers the existing lead notice", async () => {
    const { posted } = setup("contact", () =>
      jsonResponse(
        {
          code: "lead_holds_email",
          details: { lead_ids: ["lead-existing"], convertible: false },
        },
        409,
      ),
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Add as contact" }),
    );
    expect(
      await screen.findByRole("button", { name: "Open the lead" }),
    ).toBeInTheDocument();
    expect(screen.getByText("bob@buyer.test")).toBeInTheDocument();
    expect(posted).toHaveLength(1);
  });

  it("does not retry a create that collides with an existing contact", async () => {
    const { posted } = setup(
      "contact",
      () =>
        jsonResponse(
          {
            code: "duplicate_email",
            details: { existing_id: "contact-existing" },
          },
          409,
        ),
      [bob],
      true,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Add as contact" }),
    );
    await waitFor(() => expect(posted).toHaveLength(1));
    await waitFor(() =>
      expect(screen.queryByText("bob@buyer.test")).not.toBeInTheDocument(),
    );
    expect(posted).toHaveLength(1);
  });

  it("draws no card after an empty read completes", async () => {
    const { reads, view } = setup("contact", undefined, []);
    await waitFor(() => expect(reads()).toBe(1));
    expect(view.container).toBeEmptyDOMElement();
  });
});

it("keeps a malformed optional response inside its own retryable panel", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"], contact: ["read"] }),
    "GET /contacts/anchor/conversation-suggestions": () =>
      jsonResponse({ id: "wrong-shape" }),
  });
  renderWithQueryClient(
    <ConversationSuggestionsCard record={{ type: "contact", id: "anchor" }} />,
  );
  expect(
    await screen.findByRole("button", { name: "Retry" }),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Add as contact" })).toBeNull();
});

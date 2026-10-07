/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { WorkAsLeadAction } from "./contactleadaction";
import { jsonResponse, StoryProviders } from "./story-utils";

/** The backend the action talks to; `answer` decides what the create says. */
function stubBackend(
  posted: unknown[],
  answer: () => Response,
  worked: readonly { id: string }[] = [],
  lookups: string[] = [],
) {
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = request ? request.url : String(input);
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "POST" && url.includes("/leads")) {
        posted.push(
          JSON.parse(request ? await request.text() : String(init?.body)),
        );
        return answer();
      }
      if (url.includes("/me")) {
        return jsonResponse({
          user: { id: "u-me", email: "me@example.test", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: { lead: ["read", "create"] } })
            .authorization,
        });
      }
      return jsonResponse({
        data: url.includes("from_contact_id=c-ben")
          ? (lookups.push(url), worked)
          : [],
        page: { next_cursor: null, has_more: false },
      });
    },
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("Work as a lead", () => {
  it("creates the reader's lead from the contact and opens it", async () => {
    const posted: unknown[] = [];
    const lookups: string[] = [];
    stubBackend(posted, () => jsonResponse({ id: "l-new" }, 201), [], lookups);
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <WorkAsLeadAction contactId="c-ben" />
      </StoryProviders>,
    );

    const button = await screen.findByRole("button", {
      name: "Work as a lead",
    });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    await user.click(button);

    await waitFor(() => expect(window.location.hash).toBe("#/leads/l-new"));
    // The contact now has a lead, so the page asks again rather than keep
    // offering a second one from its cached answer.
    await waitFor(() => expect(lookups).toHaveLength(2));
    expect(posted).toEqual([
      {
        contact_id: "c-ben",
        owner_id: "u-me",
        status: "new",
        source: "manual",
      },
    ]);
  });

  it("opens the lead that already holds the contact rather than failing", async () => {
    stubBackend([], () =>
      jsonResponse(
        {
          status: 409,
          code: "duplicate_email",
          details: { existing_id: "l-old" },
        },
        409,
      ),
    );
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <WorkAsLeadAction contactId="c-ben" />
      </StoryProviders>,
    );

    const button = await screen.findByRole("button", {
      name: "Work as a lead",
    });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    await user.click(button);

    await waitFor(() => expect(window.location.hash).toBe("#/leads/l-old"));
  });

  it("offers the lead a contact is already worked through instead of a second one", async () => {
    const posted: unknown[] = [];
    stubBackend(posted, () => jsonResponse({ id: "l-new" }, 201), [
      { id: "l-open" },
    ]);
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <WorkAsLeadAction contactId="c-ben" />
      </StoryProviders>,
    );

    await user.click(
      await screen.findByRole("button", { name: "Open the lead" }),
    );

    await waitFor(() => expect(window.location.hash).toBe("#/leads/l-open"));
    expect(screen.queryByRole("button", { name: "Work as a lead" })).toBeNull();
    expect(posted).toEqual([]);
  });
});

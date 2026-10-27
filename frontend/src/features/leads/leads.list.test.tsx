/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LeadScreen, LeadsScreen } from "./leads";
import {
  emptyPage,
  jsonResponse,
  lead,
  render,
  SHIPPED_LEAD_ORIGINS,
  stubFetch,
  stubFetchWithMe,
} from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

// The columns picker also offers a "Status" checkbox, because the column shares
// its header text with the filter attribute's label. So a plain name match is
// ambiguous. Each click is scoped to the filter menu as it stands at that
// moment, which names the step it is on: "Filter", then the attribute.
async function pickFilter(attribute: string, value: string) {
  await userEvent.click(
    await screen.findByRole("button", { name: "Add filter" }),
  );
  const step = (name: string) => screen.getByRole("group", { name });
  await userEvent.click(
    within(step("Filter")).getByRole("button", { name: attribute }),
  );
  await userEvent.click(
    within(step(attribute)).getByRole("radio", { name: value }),
  );
}

describe("LeadsScreen — search/sort/pagination + status filter (P-14)", () => {
  it("carries the debounced search term into the next fetch", async () => {
    const { urls } = stubFetch(async () => emptyPage());
    render(<LeadsScreen />);
    const search = await screen.findByPlaceholderText("Search");

    vi.useFakeTimers();
    try {
      fireEvent.change(search, {
        target: { value: "jonas" },
      });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() =>
      expect(urls.some((url) => url.includes("q=jonas"))).toBe(true),
    );
  });

  it("sends status=contacted when the status filter is set", async () => {
    const { urls } = stubFetch(async () => emptyPage());
    render(<LeadsScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await pickFilter("Status", "Contacted");

    await waitFor(() =>
      expect(urls.some((url) => url.includes("status=contacted"))).toBe(true),
    );
  });

  it("an overdue lead wears the badge, and the Overdue view asks for sla_state=breached", async () => {
    // The badge is the first-response clock as the list shows it. A lead within
    // target carries no badge, so the list stays quiet until something needs a hand.
    const { urls } = stubFetch(async () =>
      jsonResponse({
        data: [
          {
            ...lead,
            sla_state: "breached",
            sla_deadline_at: "2026-06-01T08:00:00Z",
          },
          {
            ...lead,
            id: "l-2",
            full_name: "Otto Fischer",
            sla_state: "within_target",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<LeadsScreen />);
    await waitFor(() => expect(screen.getByText("Otto Fischer")).toBeTruthy());
    expect(screen.getAllByText("Overdue").length).toBeGreaterThan(0);
    expect(screen.queryByText("On time")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Overdue" }));
    await waitFor(() =>
      expect(urls.some((url) => url.includes("sla_state=breached"))).toBe(true),
    );
  });

  it("the lead page says when the first response is due, and when it was given", async () => {
    stubFetch(async () =>
      jsonResponse({
        ...lead,
        sla_state: "at_risk",
        sla_deadline_at: "2026-06-01T08:00:00Z",
      }),
    );
    render(<LeadScreen id="l-1" />);
    // The label already says which response, so the detail says only when.
    // The call at the head of the page states the same deadline, which is why
    // more than one element carries it.
    expect((await screen.findAllByText(/^Due /)).length).toBeGreaterThan(0);
    expect(screen.getByText("Due soon")).toBeTruthy();
  });

  it("bulk-assigns in ONE request carrying each row's own version, and names the row that refused", async () => {
    // One request, not one per row. The server owns the rule about who may
    // receive a lead and answers for each lead in the same breath. Each row
    // still carries the version the list holds. A single shared version would
    // conflict on every row but the one it came from. A row that moved under
    // the reader is reported by name rather than swallowed.
    let sent: { owner_id?: string; leads?: unknown } | null = null;
    let patchCount = 0;
    stubFetch(async (url, method, request) => {
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-9", email: "lena@x.test", display_name: "Lena F." }],
          page: { next_cursor: null },
        });
      }
      if (method === "PATCH") {
        patchCount += 1;
        return jsonResponse(lead);
      }
      if (url.includes("/leads/assign-bulk")) {
        sent = JSON.parse(await request.text());
        return jsonResponse({
          results: [
            { lead_id: "l-1", outcome: "assigned", version: 4 },
            { lead_id: "l-2", outcome: "conflict" },
          ],
        });
      }
      return jsonResponse({
        data: [
          { ...lead, version: 3 },
          { ...lead, id: "l-2", full_name: "Otto Fischer", version: 7 },
        ],
        page: { next_cursor: null, has_more: false },
      });
    });
    render(<LeadsScreen />);
    await waitFor(() => expect(screen.getByText("Otto Fischer")).toBeTruthy());
    await userEvent.click(
      screen.getByRole("checkbox", { name: "Select Jonas Petersen" }),
    );
    await userEvent.click(
      screen.getByRole("checkbox", { name: "Select Otto Fischer" }),
    );
    expect(screen.getByText("2 selected")).toBeTruthy();

    await userEvent.click(screen.getByLabelText("New owner"));
    await userEvent.click(
      await screen.findByRole("option", { name: "Lena F." }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Assign" }));

    await waitFor(() => expect(sent).not.toBeNull());
    expect(sent).toEqual({
      owner_id: "u-9",
      leads: [
        { id: "l-1", version: 3 },
        { id: "l-2", version: 7 },
      ],
    });
    // The old shape is gone, not merely unused: a per-row PATCH here would be
    // the unvalidated path the bulk endpoint exists to replace.
    expect(patchCount).toBe(0);
    // The row that refused is named, with the outcome the server reported.
    expect(await screen.findByText(/1 not applied/)).toBeTruthy();
    expect(screen.getByText(/Otto Fischer: /)).toBeTruthy();
  });

  it("says so when the whole bulk assign is refused, not just when rows are", async () => {
    // The per-row list reads outcomes, which only fill on success. A
    // destination the server refused before touching any lead produces no
    // outcomes at all. Without its own sentence, the reader presses Assign
    // and watches nothing happen.
    stubFetch(async (url, _method, _request) => {
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-9", email: "lena@x.test", display_name: "Lena F." }],
          page: { next_cursor: null },
        });
      }
      if (url.includes("/leads/assign-bulk")) {
        return jsonResponse(
          {
            title: "Unprocessable Entity",
            status: 422,
            code: "validation_error",
            detail:
              "the owner must be an active colleague you may assign work to",
            details: {
              errors: [{ field: "owner_id", code: "owner_not_assignable" }],
            },
          },
          422,
        );
      }
      return jsonResponse({
        data: [lead],
        page: { next_cursor: null, has_more: false },
      });
    });
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("checkbox", { name: "Select Jonas Petersen" }),
    );
    await userEvent.click(screen.getByLabelText("New owner"));
    await userEvent.click(
      await screen.findByRole("option", { name: "Lena F." }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Assign" }));

    expect(
      await screen.findByText(/active colleague you may assign work to/),
    ).toBeTruthy();
  });

  it("the Unassigned view asks for ownerless leads, oldest first", async () => {
    // The queue's whole point is the lead nobody has answered yet. What makes
    // one urgent is how long it has waited, so this view sorts against the
    // others, ascending by arrival.
    const { urls } = stubFetch(async () =>
      jsonResponse({
        data: [lead],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Unassigned" }));

    // Both dials in the same request, and not the New & unassigned view's
    // request, which also asks unassigned=true and also sorts by arrival. An
    // assertion satisfied by either one is satisfied by the wrong one, and
    // stays green with the sort deleted.
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("unassigned=true") &&
            !url.includes("status=") &&
            /[?&]sort=created_at(&|$)/.test(url),
        ),
      ).toBe(true),
    );
  });

  it("the New & unassigned view composes both dimensions in one ask", async () => {
    // Status is lifecycle and ownership is ownership: a New lead may already
    // have an owner, and an older Contacted one may have none. The view that
    // answers "new work nobody has picked up" has to say both.
    const { urls } = stubFetch(async () =>
      jsonResponse({
        data: [lead],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "New and unassigned" }),
    );

    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("unassigned=true") && url.includes("status=new"),
        ),
      ).toBe(true),
    );
  });

  it("fetches the next cursor page when the pager steps past the loaded page", async () => {
    const { urls } = stubFetch(async (url) => {
      if (url.includes("cursor=c1")) {
        return jsonResponse({
          data: [{ ...lead, id: "l-2", full_name: "Otto Fischer" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        data: [lead],
        page: { next_cursor: "c1", has_more: true },
      });
    });
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );

    const next = screen.getByRole("button", { name: "Next" });
    expect((next as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(next);

    await waitFor(() => expect(screen.getByText("Otto Fischer")).toBeTruthy());
    expect(urls.some((url) => url.includes("cursor=c1"))).toBe(true);
  });
});

describe("LeadsScreen — rich create (P-15)", () => {
  it("creates a manual lead owned by the current rep", async () => {
    let posted: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...lead, id: "l-new" }, 201);
      }
      return emptyPage();
    });
    const user = userEvent.setup();
    render(<LeadsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    await user.type(form.getByLabelText("Full name *"), "Otto Fischer");
    await user.type(form.getByLabelText("Email"), "otto@example.test");
    const linkedin = "https://linkedin.com/in/otto";
    await user.type(form.getByLabelText("LinkedIn URL"), linkedin);
    await user.type(form.getByLabelText("Company"), "Otto Fischer GmbH");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      full_name: "Otto Fischer",
      email: "otto@example.test",
      linkedin_url: linkedin,
      company_name: "Otto Fischer GmbH",
      owner_id: "u-9",
      status: "new",
    });
    expect(posted).not.toHaveProperty("source");
  });

  it("files the lead under the origin and note the form carries, and sends no source", async () => {
    let posted: Record<string, unknown> | null = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...lead, id: "l-new" }, 201);
      }
      if (url.endsWith("/v1/lead-origins")) {
        return jsonResponse(SHIPPED_LEAD_ORIGINS);
      }
      return emptyPage();
    });
    const user = userEvent.setup();
    render(<LeadsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    await user.type(form.getByLabelText("Full name *"), "Otto Fischer");
    await user.click(form.getByRole("combobox", { name: "Origin" }));
    await user.click(await screen.findByRole("option", { name: "Referral" }));
    await user.type(form.getByLabelText("Origin note"), "Berlin summit");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      origin: "referral",
      origin_detail: "Berlin summit",
    });
    expect(posted).not.toHaveProperty("source");
  });

  it("finds an exact email duplicate before POST and links to it", async () => {
    let postCount = 0;
    stubFetch(async (url, method) => {
      if (method === "POST" && url.includes("/leads")) {
        postCount += 1;
      }
      if (
        method === "GET" &&
        url.includes("/leads?") &&
        url.includes("q=duplicate%40example.test")
      ) {
        return jsonResponse({
          data: [
            { ...lead, id: "existing-lead", email: "duplicate@example.test" },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      return emptyPage();
    });
    render(<LeadsScreen />);
    await userEvent.click(await screen.findByTestId("new-record"));
    await userEvent.type(screen.getByLabelText("Full name *"), "Duplicate");
    await userEvent.type(
      screen.getByLabelText("Email"),
      "duplicate@example.test",
    );
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(await screen.findByText("View existing record")).toBeTruthy();
    expect(postCount).toBe(0);
    await userEvent.click(screen.getByText("View existing record"));
    expect(window.location.hash).toBe("#/leads/existing-lead");
  });

  it("sends a lead for an existing contact to the contact, and says a new opportunity is a deal", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.includes("/leads")) {
        return new Response(
          JSON.stringify({
            code: "contact_holds_email",
            status: 409,
            detail: "they are already a contact",
            details: {
              existing_id: "held-contact",
              field: "email",
              owner_notified: true,
            },
          }),
          {
            status: 409,
            headers: { "Content-Type": "application/problem+json" },
          },
        );
      }
      return emptyPage();
    });
    const user = userEvent.setup();
    render(<LeadsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    await user.type(screen.getByLabelText("Full name *"), "Mara Held");
    await user.type(screen.getByLabelText("Email"), "mara@customer.example");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(
      await screen.findByText(/open a deal on the contact/),
    ).toBeInTheDocument();
    await user.click(screen.getByText("View existing record"));
    expect(window.location.hash).toBe("#/contacts/held-contact");
  });
});

describe("LeadsScreen — archived marking (P-3)", () => {
  it("shows a Disqualified badge on a row with archived_at set", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [
          {
            ...lead,
            status: "disqualified",
            archived_at: "2026-07-01T00:00:00Z",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );
    const warned = screen
      .getAllByText("Disqualified", { selector: ".badge-label" })
      .filter((label) => label.closest(".badge.badge-warning"));
    expect(warned).toHaveLength(1);
  });
});

describe("LeadsScreen — the one ownership dial (DM-VOCAB-OWN-1)", () => {
  it("opens on my queue and sends the origin filter to the server", async () => {
    const { urls } = stubFetchWithMe(async (url) => {
      if (url.includes("/leads")) {
        return jsonResponse({
          data: [lead],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/teams")) {
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }
      return undefined;
    });
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );

    const leadReads = urls.filter((url) => url.includes("/leads?"));
    expect(leadReads.length).toBeGreaterThan(0);
    expect(leadReads.every((url) => url.includes("owner_id=u-9"))).toBe(true);

    await pickFilter("Origin", "Website");
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("/leads?") &&
            url.includes("owner_id=u-9") &&
            url.includes("origin=website"),
        ),
      ).toBe(true),
    );
  });

  it("offers the unowned queue and asks the server for it as unassigned=true", async () => {
    // The lead list binds the same owner dial the contact and company lists
    // use, through the owner_team_id and unassigned params of listLeads.
    const user = userEvent.setup();
    const { urls } = stubFetchWithMe(async (url) => {
      if (url.includes("/leads")) {
        return jsonResponse({
          data: [lead],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/teams")) {
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }
      return undefined;
    });
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );

    const owner = await screen.findByRole("group", { name: /Owner:/ });
    const valueButton = within(owner)
      .getAllByRole("button", { name: "Owned by you" })
      .find((button) => button.hasAttribute("aria-expanded"));
    if (!valueButton) throw new Error("owner value control is missing");
    await user.click(valueButton);
    await user.click(within(owner).getByRole("radio", { name: "Unassigned" }));

    await waitFor(() =>
      expect(
        urls.some(
          (u) => u.includes("/leads?") && u.includes("unassigned=true"),
        ),
      ).toBe(true),
    );
  });
});

describe("LeadsScreen — dedupe view-existing link (P-16)", () => {
  it("renders a link to the collided record on a duplicate_email 409", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.includes("/leads")) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            detail: "email already in use",
            code: "duplicate_email",
            details: { existing_id: "01X" },
          },
          409,
        );
      }
      return emptyPage();
    });
    render(<LeadsScreen />);
    await userEvent.click(await screen.findByTestId("new-record"));
    await userEvent.type(screen.getByLabelText("Full name *"), "Dup Lead");
    await userEvent.type(screen.getByLabelText("Email"), "dup@example.test");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(screen.getByText("View existing record")).toBeTruthy(),
    );
    await userEvent.click(screen.getByText("View existing record"));
    expect(window.location.hash).toBe("#/leads/01X");
  });
});

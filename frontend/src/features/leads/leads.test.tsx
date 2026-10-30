/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { LeadScreen, LeadsScreen } from "./leads";
import {
  emptyPage,
  jsonResponse,
  LEAD_GRANTS,
  lead,
  openLeadActions,
  render,
  stubFetch,
} from "./leads.testkit";

// The status/score-override/assign-to-me controls (Phase 4) resolve the
// session principal via /v1/me, which needs a workspace slug before it will
// even ask — set on every test, cleared after (mirrors automations.test.tsx).
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

// Leads (B-EP09.10a/b, §3.5 segregation): visually SEGREGATED from the
// contact graph — the ≥60/40–59/<40 score thresholds, eligibility-gated
// promote, and a lead row navigating to the LEAD detail (never the contact
// screen). Below that: the same P-14/15/16/1 shared-block wiring as contacts
// (contacts.test.tsx) and companies (companies.test.tsx) — search/sort/
// pagination + a status filter, the rich create modal (full_name/email/
// linkedin_url/company_name), the lead-360 If-Match edit
// (Promote + badges preserved), and the duplicate_email dedupe link.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const anna = {
  id: "p-1",
  full_name: "Anna Weber",
  captured_by: "human:u-1",
  source: "manual",
  version: 1,
};

describe("LeadsScreen + LeadScreen (B-EP09.10b, §3.5 segregation)", () => {
  it("names the owner on each row, the way the contacts and company lists do", async () => {
    // The column this replaced rendered "typed by a person" for every
    // human-captured row. The Contacts and Companies lists were corrected
    // while this one kept its own copy of the column: same column, same test.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { id: "u-9", display_name: "Me" },
            roles: ["rep"],
            teams: [],
            authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
          });
        }
        if (request.url.includes("/users")) {
          return jsonResponse({
            data: [
              { id: "u-9", email: "lena@x.test", display_name: "Lena F." },
            ],
            page: { next_cursor: null },
          });
        }
        return jsonResponse({
          data: [{ ...lead, owner_id: "u-9" }],
          page: { next_cursor: null },
        });
      }),
    );
    render(<LeadsScreen />);
    await waitFor(() => expect(screen.getByText("Lena F.")).toBeTruthy());
    expect(screen.queryByText(/typed by a person/i)).toBeNull();
  });

  // The queue opens on the seat's OWN leads or on every lead, and which one is
  // the server's row scope rather than the caller's role keys — reading
  // `roles` is a second reading of one policy, and a custom role, or a seeded
  // role whose scope an operator edits, opens the wrong view while the server
  // answers correctly. The assertion is the request the list makes, because
  // that is where the filter lands: a rendered row count would pass for a
  // screen that asked for everything and happened to be handed one lead.
  describe("the queue's opening view follows the row scope, not the role", () => {
    const leadRequestFor = async (
      authorization: Record<string, unknown> | undefined,
    ) => {
      const asked: string[] = [];
      vi.stubGlobal(
        "fetch",
        vi.fn(async (request: Request) => {
          if (request.url.endsWith("/v1/me")) {
            return jsonResponse({
              user: { id: "u-9", display_name: "Me" },
              // Deliberately the role that USED to decide this. It says "open
              // wide" in every case below, so the two narrow arms fail if
              // anything still reads the role instead of the scope.
              roles: ["admin"],
              teams: [],
              ...(authorization ? { authorization } : {}),
            });
          }
          // The LIST query specifically: the screen also asks /leads/settings,
          // which carries no filter and would satisfy the assertions vacuously.
          if (/\/v1\/leads\?/.test(request.url)) {
            asked.push(request.url);
          }
          return jsonResponse({ data: [lead], page: { next_cursor: null } });
        }),
      );
      render(<LeadsScreen />);
      await waitFor(() => expect(asked.length).toBeGreaterThan(0));
      return asked[0];
    };

    const objects = { lead: { read: true } };

    it("a seat scoped to its own records asks for its own", async () => {
      const url = await leadRequestFor({
        seat_type: "full",
        row_scope: "own",
        objects,
      });
      expect(url).toContain("owner_id=u-9");
    });

    it("a seat scoped to its team asks for every lead", async () => {
      const url = await leadRequestFor({
        seat_type: "full",
        row_scope: "team",
        objects,
      });
      expect(url).not.toContain("owner_id=");
    });

    it("a seat scoped to the whole installation asks for every lead", async () => {
      const url = await leadRequestFor({
        seat_type: "full",
        row_scope: "all",
        objects,
      });
      expect(url).not.toContain("owner_id=");
    });

    // Fail closed: a response carrying no authorization block at all is the
    // narrow view, never the wide one. `row_scope !== "own"` would read a
    // missing answer as permission to open on everything.
    it("a response with no authorization block asks for its own", async () => {
      const url = await leadRequestFor(undefined);
      expect(url).toContain("owner_id=u-9");
    });
  });

  it("a lead row navigates to the LEAD detail, not the contact screen", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) =>
        request.url.endsWith("/v1/me")
          ? jsonResponse({
              user: { id: "u-9", display_name: "Me" },
              roles: ["rep"],
              teams: [],
              authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
            })
          : jsonResponse({ data: [lead], page: { next_cursor: null } }),
      ),
    );
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Jonas Petersen")).toBeTruthy(),
    );
    await userEvent.click(screen.getByText("Jonas Petersen"));
    expect(window.location.hash).toBe("#/leads/l-1");
  });

  // The app shell yields its page heading on a record route (app/shell.tsx:
  // PageTitle renders nothing there, and the trail stands in the top bar), so
  // the lead's own surface is the only thing that can name this page.
  it("names the page after the lead, at heading level one", async () => {
    stubFetch(async () => jsonResponse(lead));
    render(<LeadScreen id="l-1" />);
    expect(
      await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" }),
    ).toBeTruthy();
  });

  it("the qualify dialog derives the reason from what was captured, and asks for no trigger", async () => {
    stubFetch(async () => jsonResponse(lead));
    render(<LeadScreen id="l-1" />);
    await userEvent.click(await screen.findByTestId("lead-qualify"));
    // A lead with no captured engagement is qualified by the rep's own
    // judgement — said as a sentence, not picked from a technical list.
    expect(await screen.findByText("Reason: qualified by you.")).toBeTruthy();
    expect(screen.queryByLabelText("Promotion trigger")).toBeNull();
  });

  it("the qualify dialog asks about a typed company the email cannot place, and sends the rep's choice", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({
          outcome: "create",
          typed_company: "Nordwind Labs",
        });
      }
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    const choice = await screen.findByTestId("lead-qualify-create-company");
    expect(
      screen.getByText(
        en["lead.qualify.companyUnresolved"].replace(
          "{company}",
          "Nordwind Labs",
        ),
      ),
    ).toBeTruthy();
    await user.click(choice);
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({ create_company: true });
  });

  it("the qualify dialog asks nothing about the company when the preview names none to place", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({ outcome: "create" });
      }
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    expect(await screen.findByText(en["lead.previewCreate"])).toBeTruthy();
    expect(screen.queryByTestId("lead-qualify-create-company")).toBeNull();
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({ create_company: false });
  });

  it("the qualify dialog names the captured reply as the reason and sends that trigger with its activity", async () => {
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      return jsonResponse({
        ...lead,
        status: "engaged",
        qualification_evidence: {
          trigger: "inbound_reply",
          activity_id: "a-77",
          occurred_at: "2026-08-14T09:00:00Z",
        },
      });
    });
    render(<LeadScreen id="l-1" />);
    await userEvent.click(await screen.findByTestId("lead-qualify"));
    expect(await screen.findByText(/Reason: the lead replied on/)).toBeTruthy();
    // Engaged leads start with the deal block ticked; untick it here so the
    // request is the bare promotion.
    await userEvent.click(screen.getByTestId("lead-qualify-with-deal"));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({
      trigger: "inbound_reply",
      evidence: { activity_id: "a-77" },
    });
  });

  it("qualify posts the note, stays on the lead page and says who the lead became", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({ contact: anna, merged: false, lead_id: "l-1" });
      }
      if (url.includes("/contacts/")) {
        return jsonResponse(anna);
      }
      return jsonResponse(lead);
    });
    window.location.hash = "#/leads/l-1";
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    await user.type(
      screen.getByLabelText("Evidence note (optional)"),
      "Booked via calendly",
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({
      trigger: "human_qualify",
      evidence: { note: "Booked via calendly" },
    });
    // The page stays (ADR-0119): the outcome is said here, with the contact.
    expect(window.location.hash).toBe("#/leads/l-1");
    expect(
      await screen.findByText(/Jonas Petersen is now a contact:/),
    ).toBeTruthy();
  });

  it("qualify with a deal sends the deal block with the chosen stage and the base currency", async () => {
    const user = userEvent.setup();
    let promoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads/l-1/promote")) {
        promoteBody = JSON.parse(await request.text());
        return jsonResponse({
          contact: anna,
          merged: false,
          lead_id: "l-1",
          deal_id: "d-1",
        });
      }
      if (url.endsWith("/v1/pipelines")) {
        return jsonResponse({
          data: [
            {
              id: "pl-1",
              name: "Sales",
              is_default: true,
              position: 0,
              stages: [
                {
                  id: "st-1",
                  pipeline_id: "pl-1",
                  name: "Qualified",
                  position: 1,
                  semantic: "open",
                  win_probability: 10,
                },
                {
                  id: "st-2",
                  pipeline_id: "pl-1",
                  name: "Discovery",
                  position: 2,
                  semantic: "open",
                  win_probability: 25,
                },
                {
                  id: "st-9",
                  pipeline_id: "pl-1",
                  name: "Won",
                  position: 9,
                  semantic: "won",
                  win_probability: 100,
                },
              ],
            },
          ],
          page: { has_more: false },
        });
      }
      if (url.endsWith("/v1/installation/settings")) {
        return jsonResponse({
          name: "Nordwind",
          timezone: "Europe/Berlin",
          base_currency: "EUR",
          base_currency_locked: false,
          max_upload_bytes: 1,
        });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await user.click(await screen.findByTestId("lead-qualify"));
    await user.click(screen.getByTestId("lead-qualify-with-deal"));
    await user.type(
      await screen.findByTestId("lead-qualify-deal-name"),
      "Nordwind — Pilot",
    );
    await user.type(screen.getByLabelText("Amount (EUR)"), "1200");
    await user.click(
      screen.getByRole("button", { name: "Qualify and open deal" }),
    );
    await waitFor(() => expect(promoteBody).toBeTruthy());
    expect(promoteBody).toMatchObject({
      trigger: "human_qualify",
      deal: {
        pipeline_id: "pl-1",
        stage_id: "st-1",
        name: "Nordwind — Pilot",
        amount_minor: 120000,
        currency: "EUR",
      },
    });
  });

  it("a 409 already_promoted is shown in the dialog rather than swallowed", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        const method =
          input instanceof Request ? input.method : (init?.method ?? "GET");
        if (method === "POST" && url.includes("/leads/l-1/promote")) {
          return jsonResponse(
            {
              title: "already promoted",
              code: "already_promoted",
              details: { promoted_contact_id: "p-9" },
            },
            409,
          );
        }
        if (new URL(url).pathname.endsWith("/context")) {
          return jsonResponse({
            anchor: { type: "lead", id: "l-1" },
            sections: [],
          });
        }
        if (new URL(url).pathname.endsWith("/me")) {
          return jsonResponse(meFixture({ allow: LEAD_GRANTS }));
        }
        if (
          method === "GET" &&
          (url.endsWith("/v1/connectors") ||
            new URL(url).pathname.endsWith("/activities/latest-inbound"))
        ) {
          return jsonResponse({ data: [] });
        }
        return jsonResponse(lead);
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<LeadScreen id="l-1" />);
    await userEvent.click(await screen.findByTestId("lead-qualify"));
    const dialog = within(screen.getByRole("dialog"));
    await userEvent.click(dialog.getByRole("button", { name: /^Qualify/ }));
    // The refusal is shown where the reader is; the page re-reads the lead
    // and its outcome card then says what it became.
    expect((await dialog.findByRole("alert")).textContent).toContain(
      "already promoted",
    );
  });

  it("the board moves a lead between the two live statuses, with If-Match", async () => {
    // Only new and working: promoted and disqualified are reachable through
    // their own audited verbs, and a board column for either would offer a
    // drag that ends in a 422 — or worse, imply a lead can be promoted by
    // moving a card, which is what ADR-0008's trigger set exists to prevent.
    type Patched = { body: unknown; ifMatch: string | null };
    const patched: Patched[] = [];
    stubFetch(async (input: RequestInfo | URL, method: string, request) => {
      const url = String(input);
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patched.push({
          body: await request.json(),
          ifMatch: request.headers.get("If-Match"),
        });
        return jsonResponse({ ...lead, status: "contacted" });
      }
      if (url.includes("/leads?") || url.endsWith("/leads")) {
        return jsonResponse({
          data: [{ ...lead, status: "new", version: 7 }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return emptyPage();
    });
    render(<LeadsScreen />);

    await userEvent.click(await screen.findByRole("button", { name: "Board" }));
    const card = await screen.findByText("Jonas Petersen");
    const workingColumn = screen.getByRole("region", { name: "Contacted" });

    // jsdom ships no DataTransfer, so the drag carries the same two methods
    // the handlers actually use — setData on the way out, getData on the way
    // in. Anything richer would be a mock of a thing this code never touches.
    const carried = new Map<string, string>();
    const data = {
      setData: (key: string, value: string) => carried.set(key, value),
      getData: (key: string) => carried.get(key) ?? "",
    };
    fireEvent.dragStart(card.closest("button") as HTMLElement, {
      dataTransfer: data,
    });
    fireEvent.drop(workingColumn, { dataTransfer: data });

    await waitFor(() => expect(patched.length).toBe(1));
    expect(patched[0].body).toMatchObject({ status: "contacted" });
    // The version rides the variables, so the write cannot clobber a change
    // made since this card rendered.
    expect(patched[0].ifMatch).toBe("7");
  });

  it("the board keeps the filter bar it obeys, and offers the rest of the page", async () => {
    // The board renders inside the list surface. Swapping the whole surface out
    // for it took the chips and the search with it — leaving the reader looking
    // at a narrowed answer with no way to see or change what narrowed it — and
    // showed page one while looking like the whole pipeline.
    stubFetch(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/leads?") || url.endsWith("/leads")) {
        return jsonResponse({
          data: [{ ...lead, status: "new", version: 7 }],
          page: { next_cursor: "page-2", has_more: true },
        });
      }
      return emptyPage();
    });
    render(<LeadsScreen />);

    await userEvent.click(await screen.findByRole("button", { name: "Board" }));

    // The dials the board obeys are still on screen.
    expect(screen.getByRole("button", { name: "Add filter" })).toBeTruthy();
    expect(screen.getByRole("searchbox")).toBeTruthy();
    // And it admits there is more than it is showing.
    expect(screen.getByRole("button", { name: "Load more" })).toBeTruthy();
  });

  it("edits a lead field in place, sending If-Match", async () => {
    // The Edit modal is four clicks and a context switch to fix a misspelled
    // company name. It stays for wholesale edits; the fields a rep corrects
    // while reading are corrected while reading.
    type Patched = { body: unknown; ifMatch: string | null };
    const patched: Patched[] = [];
    stubFetch(async (_url: string, method: string, request) => {
      if (method === "PATCH") {
        patched.push({
          body: await request.json(),
          ifMatch: request.headers.get("If-Match"),
        });
        return jsonResponse({ ...lead, company_name: "Nordwind GmbH" });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);

    // The row reads as its value; pressing it opens the field.
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Company" }),
    );
    const field = await screen.findByDisplayValue("Nordwind Logistik");
    await userEvent.clear(field);
    await userEvent.type(field, "Nordwind GmbH");
    await userEvent.keyboard("{Enter}");

    await waitFor(() => expect(patched.length).toBe(1));
    expect(patched[0].body).toMatchObject({ company_name: "Nordwind GmbH" });
    expect(patched[0].ifMatch).toBe("1");
  });

  it("a terminal lead's fields are read-only with the reason, not missing", async () => {
    // STATE-4a: the reason is the information. A field that simply vanished on
    // a closed lead would hide the fact the reader came for.
    stubFetch(async () =>
      jsonResponse({
        ...lead,
        status: "disqualified",
        archived_at: "2026-07-13T00:00:00Z",
      }),
    );
    render(<LeadScreen id="l-1" />);

    // The value is still shown — as text, with no control to press.
    expect(
      (await screen.findAllByText("Nordwind Logistik")).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Change Company" })).toBeNull();
    expect(screen.queryByDisplayValue("Nordwind Logistik")).toBeNull();
  });

  // `writable` is what the server's write gate would answer on a mutation. A
  // rep holding lead.update on the OBJECT was offered Edit, Promote and the
  // inline rows on a colleague's lead, and learned from the 403 after filling
  // the form in that it was never theirs to save.
  it("refuses every write on a live lead this caller may not change, from the one sentence it prints", async () => {
    stubFetch(async () =>
      jsonResponse({ ...lead, owner_id: "u-other", writable: false }),
    );
    render(<LeadScreen id="l-1" />);

    const sentence =
      "You cannot change this lead. Ask its owner to share it, or an administrator for edit access.";
    // The band says it once for the page; the inline rows each carry the same
    // sentence beside their own value, as they do on a closed lead.
    expect((await screen.findAllByText(sentence)).length).toBeGreaterThan(0);
    // Each verb is refused in place, and the id it describes itself by
    // resolves to that one sentence — a disabled button whose reason is
    // nowhere on the page is a dead button.
    await openLeadActions();
    for (const testId of ["lead-qualify", "lead-disqualify"]) {
      const control = await screen.findByTestId(testId);
      expect(control.hasAttribute("disabled")).toBe(true);
      expect(
        document.getElementById(control.getAttribute("aria-describedby") ?? "")
          ?.textContent,
      ).toBe(sentence);
    }
    // The inline rows read as text with no control to press, the same shape a
    // closed lead takes.
    expect(screen.queryByRole("button", { name: "Change Company" })).toBeNull();
  });

  it("a promoted lead keeps its page and says what the promotion did", async () => {
    // AC-leaddetail-5 (ADR-0119). The page used to redirect here, which
    // told the reader the lead had ceased to exist — untrue of a record this
    // product keeps, audits and can reverse — and left the reversal with
    // nowhere to start from. It also hid whether promotion merged into a
    // contact we already knew or created a new one.
    const promoted = {
      ...lead,
      status: "promoted",
      promoted_contact_id: "p-42",
      promoted_at: "2026-06-20T08:00:00Z",
      archived_at: "2026-06-20T08:00:00Z",
    };
    stubFetch(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (/\/records\/lead\/[^/]+\/history/.test(url)) {
        return jsonResponse({
          data: [
            {
              id: "a-1",
              actor_type: "human",
              actor_id: "human:u-9",
              action: "promote",
              occurred_at: "2026-06-20T08:00:00Z",
              after: {
                dedupe_outcome: "merged",
                trigger: "inbound_reply",
                evidence_note: "Replied asking for a quote.",
              },
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse(promoted);
    });
    render(<LeadScreen id="l-1" />);

    // It stays on the lead's own page.
    expect(await screen.findByText(en["lead.card.qualifiedAs"])).toBeTruthy();
    expect(window.location.hash).not.toBe("#/contacts/p-42");
    // And it names WHICH outcome, which is the whole reason a rep opens it.
    // Awaited: the outcome comes from the audit read, a second request that
    // lands after the lead itself.
    expect(
      await screen.findByText(en["lead.promotedMerged"], { exact: false }),
    ).toBeTruthy();
    expect(screen.getByText(/Inbound reply/)).toBeTruthy();
    expect(screen.getByText(/Replied asking for a quote\./)).toBeTruthy();
  });

  it("the promote dialog says what promotion will do before the rep commits", async () => {
    // ADR-0119: merge-into-existing vs create is the difference between
    // "my prospect is now a contact" and "my prospect was already someone we
    // knew". The preview runs the same ladder the promotion runs.
    stubFetch(async (url) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({ outcome: "merge", contact: anna });
      }
      if (url.includes("/contacts/")) {
        return jsonResponse(anna);
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await userEvent.click(await screen.findByTestId("lead-qualify"));
    expect(
      await screen.findByText(/Qualifying merges into the existing contact/),
    ).toBeTruthy();
  });

  it("a withheld merge target is never read as 'create'", async () => {
    // An absent contact on a merge means "outside your row scope", not "no
    // match": promising a new contact here would be the wrong half to guess.
    stubFetch(async (url) => {
      if (url.includes("/promote-preview")) {
        return jsonResponse({
          outcome: "merge",
          contact_withheld: true,
        });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await userEvent.click(await screen.findByTestId("lead-qualify"));
    // In the dialog: the overview's preview card reads the same answer.
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(en["lead.previewMergeWithheld"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["lead.previewCreate"])).toBeNull();
  });

  it("a promoted lead can be demoted from its own page, with a recorded reason", async () => {
    const user = userEvent.setup();
    const promoted = {
      ...lead,
      status: "promoted",
      promoted_contact_id: "p-42",
      promoted_at: "2026-06-20T08:00:00Z",
      archived_at: "2026-06-20T08:00:00Z",
    };
    let demoteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads/l-1/demote")) {
        demoteBody = JSON.parse(await request.text());
        return jsonResponse({
          lead: { ...lead, status: "contacted" },
          unwind: "reversed",
          contact_id: "p-42",
        });
      }
      if (/\/records\/lead\/[^/]+\/history/.test(url)) {
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse(promoted);
    });
    render(<LeadScreen id="l-1" />);
    // A rare verb, so it waits in the head's More menu.
    await openLeadActions(user);
    await user.click(
      await screen.findByRole("button", { name: "Reverse qualification" }),
    );
    const confirm = () =>
      screen.getByRole("button", { name: "Reverse" }) as HTMLButtonElement;
    expect(confirm().disabled).toBe(true);
    await user.type(
      screen.getByLabelText("Reason (recorded in the audit trail) *"),
      "Initial reason",
    );
    const reason = screen.getByLabelText(
      "Reason (recorded in the audit trail) *",
    );
    await user.clear(reason);
    expect(confirm().disabled).toBe(true);
    await user.type(reason, "  Promoted the wrong prospect  ");
    await user.click(confirm());
    await waitFor(() => expect(demoteBody).not.toBeNull());
    expect(demoteBody).toEqual({ reason: "Promoted the wrong prospect" });
  });

  it("a promoted lead reads as promoted, not disqualified", async () => {
    // Both closures archive the row, so a page keying its terminal sentence off
    // archived_at alone told every promoted lead it had been disqualified. The
    // redirect hid that until ADR-0119 removed it.
    stubFetch(async (input: RequestInfo | URL) => {
      if (/\/records\/lead\/[^/]+\/history/.test(String(input))) {
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        ...lead,
        status: "promoted",
        promoted_contact_id: "p-42",
        archived_at: "2026-06-20T08:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    expect(
      await screen.findByText("Qualified. This lead is read-only."),
    ).toBeTruthy();
    expect(screen.queryByText(/Disqualified — this lead/)).toBeNull();
  });

  it("asks for the promotion by verb, and reads it without a second page", async () => {
    // BOTH halves: the request names the verb, and there is exactly one of
    // them. History pages at 20 rows, so a lead worked long enough to collect
    // other audit rows carries its promotion on a page a walker has to find —
    // and a screen that still walked would pass the first half alone.
    const historyURLs: string[] = [];
    stubFetch(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (/\/records\/lead\/[^/]+\/history/.test(url)) {
        historyURLs.push(url);
        return jsonResponse({
          data: [
            {
              id: "a-99",
              actor_type: "human",
              actor_id: "human:u-9",
              action: "promote",
              occurred_at: "2026-06-20T08:00:00Z",
              after: { dedupe_outcome: "merged", trigger: "inbound_reply" },
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        ...lead,
        status: "promoted",
        promoted_contact_id: "p-42",
        archived_at: "2026-06-20T08:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    expect(
      await screen.findByText(en["lead.promotedMerged"], { exact: false }),
    ).toBeTruthy();
    expect(historyURLs).toHaveLength(1);
    expect(historyURLs[0]).toContain("action=promote");
  });

  it("reports a failed history read as failed, never as still pending", async () => {
    // `pending` outranking `failed` renders a waiting line over an error
    // nobody ever sees — which is what a walk that re-armed on failure did.
    stubFetch(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (/\/records\/lead\/[^/]+\/history/.test(url)) {
        return new Response(JSON.stringify({ title: "Server Error" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      return jsonResponse({
        ...lead,
        status: "promoted",
        promoted_contact_id: "p-42",
        archived_at: "2026-06-20T08:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    expect(
      await screen.findByText("Qualified. This lead is read-only."),
    ).toBeTruthy();
    // The outcome line never claims a result it could not read.
    expect(
      screen.queryByText(en["lead.promotedMerged"], { exact: false }),
    ).toBeNull();
  });

  it("re-reads the history after promoting, so the new audit row is not missed", async () => {
    // A reader who opened the History tab BEFORE promoting holds a cached last
    // page saying there is nothing more to fetch. Promotion writes the audit
    // row the panel reads its outcome from, so without invalidating that cache
    // the panel walks no further and reports the outcome unavailable while the
    // row sits one page away.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const invalidated: unknown[] = [];
    const realInvalidate = client.invalidateQueries.bind(client);
    client.invalidateQueries = ((filters?: { queryKey?: unknown }) => {
      invalidated.push(filters?.queryKey);
      return realInvalidate(filters as never);
    }) as typeof client.invalidateQueries;

    stubFetch(async (input: RequestInfo | URL, method: string) => {
      const url = String(input);
      if (method === "POST" && url.includes("/promote")) {
        return jsonResponse({
          contact: { id: "p-42", full_name: "Jonas Petersen" },
          merged: true,
          lead_id: "l-1",
        });
      }
      if (/\/records\/lead\/[^/]+\/history/.test(url)) {
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse(lead);
    });
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <LeadScreen id="l-1" />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await userEvent.click(await screen.findByTestId("lead-qualify"));
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Qualify/,
      }),
    );

    await waitFor(() =>
      expect(
        invalidated.some(
          (key) =>
            Array.isArray(key) &&
            key[0] === "record-history" &&
            key[1] === "lead",
        ),
      ).toBe(true),
    );
  });

  it("says it cannot tell the outcome rather than guessing 'created'", async () => {
    // An empty, unreadable, or unrecognised audit row is not a merge and not a
    // creation. Reporting one would be a confident claim about something
    // nobody recorded — and "created" is the wrong half to guess, because it
    // tells a rep no duplicate check happened.
    stubFetch(async (input: RequestInfo | URL) => {
      if (/\/records\/lead\/[^/]+\/history/.test(String(input))) {
        return jsonResponse({
          data: [
            {
              id: "a-1",
              actor_type: "human",
              actor_id: "human:u-9",
              action: "promote",
              occurred_at: "2026-06-20T08:00:00Z",
              after: { dedupe_outcome: "something_new" },
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        ...lead,
        status: "promoted",
        promoted_contact_id: "p-42",
        archived_at: "2026-06-20T08:00:00Z",
      });
    });
    render(<LeadScreen id="l-1" />);

    expect(
      await screen.findByText(en["lead.promotedOutcomeUnavailable"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["lead.promotedCreated"])).toBeNull();
  });

  it("promote is disabled for an ineligible lead, and the button says why", async () => {
    // A LIVE lead with no email: ineligible, but still on screen. A promoted
    // lead is terminal and carries no promote control at all, so it cannot
    // stand in for "ineligible" (ADR-0119).
    stubFetch(async () => jsonResponse({ ...lead, email: null }));
    render(<LeadScreen id="l-1" />);
    const button = await screen.findByTestId("lead-qualify");
    await waitFor(() => expect(button).toBeDisabled());
    // The reason is wired to the control with aria-describedby, not stuffed
    // into a title a screen reader never announces on a disabled button.
    expect(button).toHaveAccessibleDescription(
      "Requires an email address and an open status.",
    );
  });
});

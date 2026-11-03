/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { LeadScreen, promoteEligible, scoreTone, terminalBadge } from "./leads";
import {
  jsonResponse,
  LEAD_GRANTS,
  lead,
  openLeadActions,
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

describe("score thresholds (AC-leads colour bands)", () => {
  it("maps ≥60 accent-strong, 40–59 medium, <40 low", () => {
    expect(scoreTone(60)).toBe("success");
    expect(scoreTone(95)).toBe("success");
    expect(scoreTone(59)).toBe("warning");
    expect(scoreTone(40)).toBe("warning");
    expect(scoreTone(39)).toBeUndefined();
  });
});

describe("promote eligibility gate", () => {
  it("requires an open status and an email", () => {
    expect(promoteEligible(lead)).toBe(true);
    expect(promoteEligible({ ...lead, status: "promoted" })).toBe(false);
    expect(promoteEligible({ ...lead, status: "disqualified" })).toBe(false);
    expect(promoteEligible({ ...lead, email: null })).toBe(false);
  });
});

describe("LeadScreen — edit with If-Match (P-1)", () => {
  it("PATCHes /leads/{id} with If-Match:<version> and only the update fields", async () => {
    let patchHeader: string | null = null;
    let patchBody: unknown = null;
    stubFetch(async (_url, method, request) => {
      if (method === "PATCH") {
        patchHeader = request.headers.get("If-Match");
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...lead, title: "VP Sales", version: 2 });
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);

    // The lead has no title, so About folds the field away until asked.
    await userEvent.click(
      await screen.findByRole("button", {
        name: /^Show \d+ empty fields?$/,
      }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Title" }),
    );
    const title = await screen.findByLabelText("Title");
    await userEvent.type(title, "VP Sales{Enter}");

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchHeader).toBe("1");
    expect(patchBody).toMatchObject({ title: "VP Sales" });
    expect(patchBody).not.toHaveProperty("status");
    expect(patchBody).not.toHaveProperty("score");
  });

  it("reads where the lead came from and how the row was added as two facts", async () => {
    stubFetch(async (url) => {
      if (url.endsWith("/v1/lead-origins")) {
        return jsonResponse(SHIPPED_LEAD_ORIGINS);
      }
      return jsonResponse({
        ...lead,
        origin: "referral",
        origin_label: "Referral",
        origin_detail: "Berlin summit",
        source: "mirror:csv:pipedrive-export:lead:l-7",
        imported_from: "pipedrive-export",
      });
    });
    render(<LeadScreen id="l-1" />);

    const origin = await screen.findByText("Origin");
    expect(origin.closest("div")).toHaveTextContent("Referral");
    expect(screen.getByText("Berlin summit")).toBeTruthy();
    const added = screen.getByText("Added via");
    expect(added.closest("div")).toHaveTextContent(
      "Imported from pipedrive-export",
    );
    expect(screen.queryByText(/mirror:csv/)).toBeNull();
  });

  it("moves the origin with a PATCH that carries the origin and no source", async () => {
    let patchBody: Record<string, unknown> | null = null;
    stubFetch(async (url, method, request) => {
      if (method === "PATCH") {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...lead, origin: "partner", version: 2 });
      }
      if (url.endsWith("/v1/lead-origins")) {
        return jsonResponse(SHIPPED_LEAD_ORIGINS);
      }
      return jsonResponse(lead);
    });
    render(<LeadScreen id="l-1" />);
    await userEvent.click(
      await screen.findByRole("button", { name: /^Show \d+ empty fields?$/ }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Origin" }),
    );
    // The picker opens on the click that started editing.
    await userEvent.click(
      await screen.findByRole("option", { name: "Partner" }),
    );

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ origin: "partner" });
    expect(patchBody).not.toHaveProperty("source");
  });

  it("preserves the Qualify button and reads the score, status and company", async () => {
    stubFetch(async () => jsonResponse(lead));
    render(<LeadScreen id="l-1" />);
    expect(await screen.findByTestId("lead-qualify")).toHaveTextContent(
      "Qualify",
    );
    // The score is the ring in How it stands.
    expect(
      (await screen.findAllByText("72")).some((el) =>
        el.classList.contains("standing-ring-score"),
      ),
    ).toBe(true);
    // The status word is the ladder's own, never the raw enum: a German
    // reader would otherwise meet "contacted" beside "In Bearbeitung".
    expect(
      screen
        .getAllByText("Contacted")
        .some((el) => el.classList.contains("stage-ladder-name")),
    ).toBe(true);
    expect(
      screen
        .getAllByText("Nordwind Logistik")
        .some((el) => el.classList.contains("stat-card-value")),
    ).toBe(true);
  });
});

describe("LeadScreen — disqualify (P-3)", () => {
  it("disqualify asks for a reason from the administered list, DELETEs with it, and keeps the page", async () => {
    const user = userEvent.setup();
    let deleteBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "DELETE" && url.includes("/leads/l-1")) {
        deleteBody = JSON.parse(await request.text());
        return jsonResponse({
          ...lead,
          status: "disqualified",
          archived_at: "2026-07-13T00:00:00Z",
          disqualify_reason: "Bad timing",
        });
      }
      if (url.endsWith("/v1/lead-disqualify-reasons")) {
        return jsonResponse({
          data: [
            {
              id: "r-1",
              label: "Not a good fit",
              sort_order: 10,
              active: true,
              system: true,
              lead_count: 0,
              version: 1,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
            {
              id: "r-2",
              label: "Bad timing",
              sort_order: 20,
              active: true,
              system: true,
              lead_count: 0,
              version: 1,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
            {
              id: "r-3",
              label: "Retired",
              sort_order: 30,
              active: false,
              system: false,
              lead_count: 0,
              version: 1,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
          ],
        });
      }
      return jsonResponse(lead);
    });
    window.location.hash = "#/leads/l-1";
    render(<LeadScreen id="l-1" />);

    await openLeadActions(user);
    await user.click(screen.getByTestId("lead-disqualify"));
    // Nothing is sent without a reason, and the button says so. The confirm is
    // named inside the dialog rather than by a test id. The trigger carries the
    // same word, so the dialog tells the two apart, as it does for a reader.
    const dialog = within(screen.getByRole("dialog"));
    const confirm = dialog.getByRole("button", {
      name: "Disqualify",
    }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    await pickOption(user, screen.getByLabelText("Reason *"), "Bad timing");
    expect(screen.queryByRole("option", { name: "Retired" })).toBeNull();
    await user.type(
      screen.getByLabelText("Note (optional)"),
      "Call back in Q4",
    );
    await user.click(dialog.getByRole("button", { name: "Disqualify" }));

    await waitFor(() => expect(deleteBody).toBeTruthy());
    expect(deleteBody).toEqual({ reason_id: "r-2", note: "Call back in Q4" });
    expect(window.location.hash).toBe("#/leads/l-1");
  });
});

// The address may name the verb the reader arrived to perform. A rep sent here
// to log a call attempt lands on the header's Log activity drawer, open and set
// to Call. A rep who opened the lead without one finds the drawer closed, as
// every other record header offers it.
describe("LeadScreen — arriving to log a call", () => {
  it("opens the header's Log activity drawer on a call when the address asks for one", async () => {
    stubFetch(async () => jsonResponse(lead));
    window.location.hash = "#/leads/l-1?action=call";
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getByLabelText("Type").textContent).toContain("Call"),
    );
  });

  it("leaves the drawer closed for a reader who only opened the lead, open on a note from the header verb", async () => {
    stubFetch(async () => jsonResponse(lead));
    window.location.hash = "#/leads/l-1";
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("button", { name: "Log activity" });
    expect(screen.queryByLabelText("Type")).toBeNull();

    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "Log activity" })
          .hasAttribute("disabled"),
      ).toBe(false),
    );
    await userEvent.click(screen.getByRole("button", { name: "Log activity" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Type").textContent).toContain("Note"),
    );
  });

  // A link pressed on the record the reader is already on changes the address
  // and nothing else, with no remount. So the drawer opens on the kind the
  // address now names rather than staying shut.
  it("opens the drawer on a call the address asks for after the page is already open", async () => {
    stubFetch(async () => jsonResponse(lead));
    window.location.hash = "#/leads/l-1";
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("button", { name: "Log activity" });
    expect(screen.queryByLabelText("Type")).toBeNull();

    // What a second link does: the hash moves under a mounted screen. The
    // event is dispatched rather than waited for, as jsdom delivers its own a
    // task later. The assertion is about the render, not the delay.
    act(() => {
      window.location.hash = "#/leads/l-1?action=call";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    await waitFor(() =>
      expect(screen.getByLabelText("Type").textContent).toContain("Call"),
    );
  });
});

describe("LeadScreen — status control (P-12)", () => {
  it("the ladder PATCHes the step the rep clicks, with If-Match", async () => {
    let patchHeader: string | null = null;
    let patchBody: unknown = null;
    stubFetchWithMe(async (url, method, request) => {
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patchHeader = request.headers.get("If-Match");
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...lead, status: "contacted", version: 2 });
      }
      return undefined;
    });
    render(<LeadScreen id="l-1" />);

    // The fixture lead is already contacted; the next rung up is the click.
    await waitFor(() =>
      expect(screen.getByTestId("lead-step-engaged")).toBeEnabled(),
    );
    // Where the lead stands is a marker, not a control. The ladder offers no
    // way to set a lead to the step it is already on, rather than offering one
    // and refusing it.
    const here = document.querySelector("[aria-current='step']");
    expect(here?.textContent).toContain("Contacted");
    expect(here?.querySelector("button")).toBeNull();
    await userEvent.click(screen.getByTestId("lead-step-engaged"));

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchHeader).toBe("1");
    expect(patchBody).toMatchObject({ status: "engaged" });
  });

  it("freezes the ladder on a promoted/disqualified lead, with the reason", async () => {
    stubFetchWithMe(async () =>
      jsonResponse({
        ...lead,
        status: "promoted",
        archived_at: "2026-07-13T00:00:00Z",
      }),
    );
    render(<LeadScreen id="l-1" />);

    await screen.findByTestId("lead-step-new");
    for (const step of ["new", "contacted", "engaged", "disqualified"]) {
      const control = screen.getByTestId(
        `lead-step-${step}`,
      ) as HTMLButtonElement;
      expect(control.disabled).toBe(true);
    }
    // The step it is ON draws no control at all, so there is nothing to freeze.
    const here = document.querySelector("[aria-current='step']");
    expect(here?.textContent).toContain("Qualified");
    expect(here?.querySelector("button")).toBeNull();
  });
});

describe("LeadScreen — score explain + override (P-10)", () => {
  it("shows Override score for a non-overridden lead; submit requires a reason and PATCHes score + reason", async () => {
    let patchBody: unknown = null;
    stubFetchWithMe(async (url, method, request) => {
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({
          ...lead,
          score: 90,
          score_override_reason: "Strong buying signal",
          score_computed: 72,
          version: 2,
        });
      }
      return undefined;
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Override score" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Override score" }),
    );

    const submit = screen.getByRole("button", { name: "Save override" });
    expect((submit as HTMLButtonElement).disabled).toBe(true);

    const scoreInput = screen.getByRole("spinbutton", { name: "Score" });
    const reasonInput = screen.getByLabelText("Reason");
    await userEvent.clear(scoreInput);
    await userEvent.type(scoreInput, "90");
    await userEvent.type(reasonInput, "Strong buying signal");
    expect((submit as HTMLButtonElement).disabled).toBe(false);

    await userEvent.click(submit);

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({
      score: 90,
      score_override_reason: "Strong buying signal",
    });
  });

  it("disables Save override for an out-of-range or non-integer score even with a reason filled in", async () => {
    let patchBody: unknown = null;
    stubFetchWithMe(async (url, method, request) => {
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...lead, version: 2 });
      }
      return undefined;
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Override score" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Override score" }),
    );

    const submit = screen.getByRole("button", { name: "Save override" });
    const scoreInput = screen.getByRole("spinbutton", { name: "Score" });
    const reasonInput = screen.getByLabelText("Reason");
    await userEvent.type(reasonInput, "Strong buying signal");

    await userEvent.clear(scoreInput);
    await userEvent.type(scoreInput, "150");
    expect((submit as HTMLButtonElement).disabled).toBe(true);

    await userEvent.clear(scoreInput);
    await userEvent.type(scoreInput, "-5");
    expect((submit as HTMLButtonElement).disabled).toBe(true);

    await userEvent.clear(scoreInput);
    await userEvent.type(scoreInput, "90.5");
    expect((submit as HTMLButtonElement).disabled).toBe(true);

    await userEvent.clear(scoreInput);
    await userEvent.type(scoreInput, "90");
    expect((submit as HTMLButtonElement).disabled).toBe(false);

    await userEvent.click(submit);
    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ score: 90 });
  });

  it("shows the reason + machine value and Clear override for an overridden lead", async () => {
    let patchBody: unknown = null;
    const overridden = {
      ...lead,
      score: 90,
      score_override_reason: "Strong buying signal",
      score_computed: 72,
    };
    stubFetchWithMe(async (url, method, request) => {
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...lead, score: 72, version: 2 });
      }
      return jsonResponse(overridden);
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getByText(/Strong buying signal/)).toBeTruthy(),
    );
    expect(screen.getByText(/72/)).toBeTruthy();
    const clear = screen.getByRole("button", { name: "Clear override" });
    await userEvent.click(clear);

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ score: null });
  });
});

describe("LeadScreen — owner display + assign to me (P-11)", () => {
  // The server refuses a PATCH against a lead nobody owns, as an ownerless row
  // is nobody's to change. So this asserts the claim, not the patch. A stub
  // answering the patch accepts a request the real backend rejects with 403.
  it("shows Unassigned and taking an unowned lead yourself goes through the claim door", async () => {
    let claimed = false;
    let patched = false;
    stubFetchWithMe(async (url, method) => {
      if (method === "POST" && url.includes("/records/lead/l-1/claim")) {
        claimed = true;
        return jsonResponse({
          record_type: "lead",
          record_id: "l-1",
          owner_id: "u-9",
        });
      }
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patched = true;
        return jsonResponse({ ...lead, owner_id: "u-9", version: 2 });
      }
      if (url.includes("/users")) {
        // The viewer is an ordinary roster entry, and the picker offers them
        // first rather than a separate button doing it.
        return jsonResponse({
          data: [{ id: "u-9", display_name: "Me" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return undefined;
    }, "u-9");
    render(<LeadScreen id="l-1" />);

    await waitFor(() => expect(screen.getByText("Unassigned")).toBeTruthy());
    // One control, not a self-assign button beside a picker: the viewer is
    // its first option (ADR-0108).
    await userEvent.click(
      await screen.findByRole("button", { name: "Assign" }),
    );
    await userEvent.click(
      await screen.findByRole("combobox", { name: "Assign this lead to" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Assign to me" }),
    );

    await waitFor(() => expect(claimed).toBe(true));
    expect(patched).toBe(false);
  });

  // The fixture above says `writable: true`, which an ownerless lead is not.
  // The write arm refuses a row nobody owns, and the server answers the read
  // with `writable: false`. A test that only renders the writable fixture
  // cannot see the picker shut for every ordinary seat.
  it("offers assignment on an unowned lead the reader may not otherwise write", async () => {
    stubFetchWithMe(async (url) => {
      if (url.includes("/leads/l-1")) {
        return jsonResponse({ ...lead, owner_id: null, writable: false });
      }
      if (url.includes("/users")) {
        return jsonResponse({
          data: [{ id: "u-9", display_name: "Me" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return undefined;
    }, "u-9");
    render(<LeadScreen id="l-1" />);

    await waitFor(() => expect(screen.getByText("Unassigned")).toBeTruthy());
    const assign = await screen.findByRole("button", { name: "Assign" });
    expect(assign.hasAttribute("disabled")).toBe(false);
  });

  // Dropping the per-row half of the write answer must not drop the other two.
  // A read seat cannot be handed work, and the server refuses its assignment.
  // So an enabled control here would only ever fail in the reader's face.
  it("offers no enabled assignment to a seat that may not write leads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.endsWith("/v1/connectors")) {
          return jsonResponse({ data: [] });
        }
        if (request.url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { id: "u-9", display_name: "Me" },
            roles: ["read_only"],
            teams: [],
            authorization: meFixture({ seat: "read", allow: LEAD_GRANTS })
              .authorization,
          });
        }
        if (request.url.includes("/leads/l-1")) {
          return jsonResponse({ ...lead, owner_id: null, writable: false });
        }
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }),
    );
    render(<LeadScreen id="l-1" />);

    await waitFor(() => expect(screen.getByText("Unassigned")).toBeTruthy());
    const assign = await screen.findByRole("button", { name: "Assign" });
    expect(assign.hasAttribute("disabled")).toBe(true);
  });

  it("hides Assign to me when the lead is already owned by the current user", async () => {
    const { urls } = stubFetchWithMe(
      async () => jsonResponse({ ...lead, owner_id: "u-9" }),
      "u-9",
    );
    render(<LeadScreen id="l-1" />);

    // LeadScreen subscribes to the /me probe up front, so a lead the viewer
    // owns settles to the "You" owner line once /me resolves. Waiting on that,
    // not the transient raw owner id, makes the self-assign assertion reliable.
    await waitFor(() => expect(screen.getByText("You")).toBeTruthy());
    await waitFor(() =>
      expect(urls.some((url) => url.endsWith("/v1/me"))).toBe(true),
    );
    expect(screen.queryByRole("button", { name: "Assign to me" })).toBeNull();
  });
});

describe("LeadScreen — History tab", () => {
  it("shows a History tab whose Changes cut lists the record's own audit", async () => {
    // The default cut is All, the same chronology the contact and the account
    // open on. The audit row here only surfaces once the reader narrows to
    // Changes. leadhistory.test.tsx covers the rest of the cut.
    stubFetchWithMe(async (url) => {
      if (url.includes("/history")) {
        return jsonResponse({
          data: [
            {
              id: "h1",
              actor_type: "human",
              actor_id: "u1",
              actor_name: "Demo Admin",
              action: "update",
              occurred_at: "2026-07-13T10:00:00Z",
              summary: "Lead score changed",
            },
          ],
          page: { next_cursor: null },
        });
      }
      return undefined;
    });
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /history/i })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: /history/i }));
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));

    await screen.findByText(
      translate("en", "history.verb.update", { subject: "Demo Admin" }),
    );
    // The identity header (name) stays visible on the History tab, not just
    // the overview. It sits above the tab switch, like the persistent
    // RecordView header of a contact, company or deal. It is named by role
    // rather than by text. The rail's details grid sits outside the tab switch
    // too and carries the same name in its editable Full name row. That is the
    // page working as intended, not a second header.
    expect(
      screen.getByRole("heading", { level: 1, name: lead.full_name }),
    ).toBeTruthy();
  });

  it("keeps the Overview tab's own content, without a second History section under it", async () => {
    stubFetchWithMe(async () => undefined);
    render(<LeadScreen id="l-1" />);

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { level: 1, name: lead.full_name }),
      ).toBeTruthy(),
    );
    // The chronology is the History tab's own body. Overview draws its cards
    // and nothing else under them, not the record's whole history a second
    // time beneath the ladder and the call.
    expect(document.querySelector(".record-timeline")).toBeNull();
    expect(screen.queryByRole("button", { name: "Changes" })).toBeNull();
  });
});

describe("terminalBadge (archived/terminal labelling)", () => {
  it("labels disqualified and promoted distinctly and leaves open leads unbadged", () => {
    expect(terminalBadge({ status: "disqualified" })).toEqual({
      label: "lead.disqualified",
      tone: "warning",
    });
    // A promoted lead is archived, and reads "Archived", never "Disqualified".
    expect(terminalBadge({ status: "promoted" })).toEqual({
      label: "record.archived",
      tone: "warning",
    });
    expect(terminalBadge({ status: "new" })).toBeNull();
    expect(terminalBadge({ status: "contacted" })).toBeNull();
  });
});

describe("LeadScreen — the header's Email verb", () => {
  it("carries the same Email verb every record header does", async () => {
    stubFetchWithMe(async () => jsonResponse(lead));
    render(<LeadScreen id="l-1" />);
    const verb = await screen.findByRole("button", { name: "Email" });
    expect(verb.querySelector(".lucide-mail")).toBeTruthy();
    expect(verb.hasAttribute("disabled")).toBe(false);
  });

  it("opens the composer on the lead from its address in About, already addressed", async () => {
    // The address is a button into the product's composer, not a link for
    // the reader's mail client. A message sent through `mailto:` is never
    // filed on the lead. The composer is offered because the reader has a
    // mailbox to send from.
    stubFetchWithMe(async () => jsonResponse(lead), "u-9", {
      data: [{ id: "g1", provider: "gmail", status: "connected", scopes: [] }],
    });
    render(<LeadScreen id="l-1" />);
    const address = await screen.findByRole("button", {
      name: "jonas@nordwind.example",
    });
    expect(address.hasAttribute("href")).toBe(false);
    await userEvent.click(address);

    const dialog = await screen.findByRole("dialog", {
      name: /New email/,
    });
    expect(
      await within(dialog).findByRole("button", {
        name: "Remove jonas@nordwind.example",
      }),
    ).toBeTruthy();
  });
});

// A surface that met this lead while creating a contact sends the reader here.
// The Qualify dialog is already open.
describe("LeadScreen — arriving to qualify", () => {
  it("opens the Qualify dialog when the address asks for it", async () => {
    stubFetch(async () => jsonResponse(lead));
    window.location.hash = "#/leads/l-1?action=qualify";
    render(<LeadScreen id="l-1" />);
    expect(
      await screen.findByRole("dialog", { name: /^Qualify / }),
    ).toBeTruthy();
  });
});

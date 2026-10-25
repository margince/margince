/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { RequestsPanel } from "../settings/settings.privacy";
import { ConsentPurposesCard } from "./privacy.purposes";
import {
  choose,
  DSRS,
  draftClose,
  findDsrRow,
  jsonResponse,
  render,
  stubRoutes,
} from "./privacy.testkit";

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("ConsentPurposesCard", () => {
  it("lists purposes and marks the ones needing double opt-in", async () => {
    stubRoutes();
    render(<ConsentPurposesCard />);
    const marketing = await screen.findByRole("row", { name: /Marketing/ });
    expect(within(marketing).getByText("Double opt-in")).toBeInTheDocument();
    const transactional = screen.getByRole("row", { name: /Deal messages/ });
    expect(within(transactional).queryByText("Double opt-in")).toBeNull();
  });

  // G-3
  it("creates a purpose", async () => {
    const sent = stubRoutes({
      "POST /consent-purposes": () =>
        jsonResponse(
          {
            id: "p3",
            key: "events",
            label: "Events",
            requires_double_opt_in: false,
            created_at: "2026-07-15T00:00:00Z",
          },
          201,
        ),
    });
    render(<ConsentPurposesCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /add purpose/i }),
    );
    await userEvent.type(screen.getByLabelText(/key/i), "events");
    await userEvent.type(screen.getByLabelText(/label/i), "Events");
    await userEvent.click(
      screen.getByRole("button", { name: /create purpose/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /consent-purposes"),
      ).toHaveLength(1),
    );
    // The onSuccess invalidation refetches the purposes GET, appending to the
    // same `sent` array — filter for the POST specifically rather than
    // trusting it stayed last.
    const posts = sent.filter((s) => s.key === "POST /consent-purposes");
    expect(posts.at(-1)?.body).toEqual({
      key: "events",
      label: "Events",
      requires_double_opt_in: false,
    });
  });

  // A purpose has no PATCH and no DELETE — say so before they commit, not after.
  it("warns that a purpose cannot be renamed or removed", async () => {
    stubRoutes();
    render(<ConsentPurposesCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /add purpose/i }),
    );
    expect(
      screen.getByText(/cannot be renamed or removed/i),
    ).toBeInTheDocument();
  });

  it("refuses to submit without a key and a label", async () => {
    stubRoutes();
    render(<ConsentPurposesCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /add purpose/i }),
    );
    expect(
      screen.getByRole("button", { name: /create purpose/i }),
    ).toBeDisabled();
  });

  it("surfaces a create failure inline without losing the typed values", async () => {
    stubRoutes({
      "POST /consent-purposes": () =>
        jsonResponse(
          { status: 409, code: "conflict", detail: 'purpose "news": conflict' },
          409,
        ),
    });
    render(<ConsentPurposesCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /add purpose/i }),
    );
    await userEvent.type(screen.getByLabelText(/key/i), "news");
    await userEvent.type(screen.getByLabelText(/label/i), "Dupe");
    await userEvent.click(
      screen.getByRole("button", { name: /create purpose/i }),
    );
    expect(await screen.findByText(/"news": conflict/)).toBeInTheDocument();
    expect(screen.getByLabelText(/key/i)).toHaveValue("news");
  });

  // Appending to the registry asks `consent_config:create`, which a rep does
  // not hold, so their card carries no write control at all. Withheld, not
  // absent: the card keeps its place and says which of the two it is, or the
  // missing button reads as a broken one.
  it("states its read-only posture to a seat that cannot add a purpose", async () => {
    stubRoutes({
      "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
    });
    render(<ConsentPurposesCard />);
    const posture = await screen.findByText(
      /adding a purpose needs a permission/i,
    );
    // In the card that holds the registry, so the posture reads as its own.
    const card = posture.closest(".panel");
    expect(card).not.toBeNull();
    expect(
      within(card as HTMLElement).getByRole("row", { name: /Marketing/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /add purpose/i }),
    ).not.toBeInTheDocument();
    // Withholding the write never withholds the read — the registry a rep
    // consults when tagging a consent is still on screen.
    expect(screen.getByText(/Marketing/)).toBeInTheDocument();
  });

  // The other direction, without which the assertion above passes on a card
  // that shows the line to everybody: this seat holds `consent_config:create`,
  // so the posture is not its posture and the sentence would be a false
  // statement.
  it("withholds the read-only line from a seat that can add a purpose", async () => {
    stubRoutes({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["ops"],
            allow: { consent_config: ["create"] },
          }),
        ),
    });
    render(<ConsentPurposesCard />);
    expect(
      await screen.findByRole("button", { name: /add purpose/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/adding a purpose needs a permission/i),
    ).not.toBeInTheDocument();
  });
});

describe("PrivacyInboxCard", () => {
  it("withholds the queue from a reader without the grant instead of asking for it", async () => {
    // The rows name the contacts who exercised an Art. 15/17 right, so the read
    // asks `privacy_request:read`. A seat reaching this page for the consent
    // registry beside it must find the card in its place saying why it is empty
    // — an absent card would read as "no requests", a different claim entirely.
    const sent = stubRoutes({
      "GET /me": () => jsonResponse(meFixture({ roles: ["ops"] })),
    });
    render(<RequestsPanel />);
    await screen.findByText(/subject requests needs a permission/i);
    expect(screen.queryByText(/anna@acme.test/)).not.toBeInTheDocument();
    // And it never issued the call the server would only refuse.
    expect(
      sent.some((entry) => entry.key === "GET /data-subject-requests"),
    ).toBe(false);
  });

  // Reading the queue and WORKING it are different grants, and the server asks
  // them separately: consent/dsr.go's UpdateDSR wants `privacy_request:update`
  // where the list wants `read`. A delegated reader used to be shown every
  // transition button and refused by each one.
  it("offers no transition to a reader who may see the queue but not work it", async () => {
    stubRoutes({
      "GET /data-subject-requests": () => jsonResponse(DSRS),
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["ops"],
            allow: { privacy_request: ["read"], contact: ["update"] },
          }),
        ),
    });
    render(<RequestsPanel />);

    // The OPEN request, not the fulfilled one: a terminal request offers no
    // transition to anybody, so asserting their absence on it would pass
    // whatever the grant said.
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button"));

    // …and carries no verb that would 403. Scoped to `.dsr-actions`, the
    // container the transitions live in: a row's summary TOGGLE carries the
    // same status words as badges, so a name-only query matches four buttons
    // that were never verbs.
    expect(document.querySelectorAll(".dsr-actions button")).toHaveLength(0);
  });

  // Opening a request is a THIRD grant: the POST writes the contact it names, so
  // consent/dsr.go's CreateDSR asks `contact:update` rather than anything on the
  // privacy object at all.
  it("offers no New request to a reader who may work the queue but not write a contact", async () => {
    stubRoutes({
      "GET /data-subject-requests": () => jsonResponse(DSRS),
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["ops"],
            allow: { privacy_request: ["read", "update"] },
          }),
        ),
    });
    render(<RequestsPanel />);

    // The queue answered, so this is about the missing grant and not about a
    // card that never rendered.
    expect(await screen.findByText(/anna@acme.test/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /new request/i }),
    ).not.toBeInTheDocument();
  });

  // The mirror of both: the full set of grants offers both verbs, so the two
  // refusals above are about what is missing rather than about controls nobody
  // is ever shown.
  it("offers the verbs to a reader holding every grant behind them", async () => {
    stubRoutes({ "GET /data-subject-requests": () => jsonResponse(DSRS) });
    render(<RequestsPanel />);

    expect(
      await screen.findByRole("button", { name: /new request/i }),
    ).toBeInTheDocument();
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button"));
    expect(
      document.querySelectorAll(".dsr-actions button").length,
    ).toBeGreaterThan(0);
  });

  it("binds the status filter server-side, never a client re-slice", async () => {
    const sent = stubRoutes({
      "GET /data-subject-requests": () => jsonResponse(DSRS),
    });
    render(<RequestsPanel />);
    await screen.findByText(/anna@acme.test/);
    await userEvent.click(screen.getByRole("button", { name: /^open$/i }));
    await waitFor(() =>
      expect(sent.some((s) => s.url.includes("status=open"))).toBe(true),
    );
    // Both rows still came back from the stub; a client re-slice would have
    // hidden the fulfilled one without ever asking the server.
    expect(
      sent.filter((s) => s.key === "GET /data-subject-requests").length,
    ).toBeGreaterThan(1);
  });

  // A request opens beside the queue. Its siblings and the facet bar stay
  // loaded behind the drawer, so closing it returns the officer to their place.
  it("opens a request beside the queue rather than in place of it", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    expect(
      screen.getByRole("dialog", { name: "8f3a-contact-uuid" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/anna@acme.test/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^open$/i })).toBeInTheDocument();
  });

  // FIX-1: due_at is a statutory deadline. Rendered in a hardcoded
  // Europe/Berlin it shows the wrong calendar day to anyone outside CET —
  // this due_at is 2026-08-01T00:00:00Z, which is 1 Aug in Berlin (+02:00)
  // but still 31 Jul in New York.
  it("renders the due date in the viewer's timezone, not a hardcoded one", async () => {
    // The card asks the platform which zone the viewer is in; pretend it's
    // New York. formatDate takes the zone as an argument and doesn't consult
    // resolvedOptions, so this spy only redirects the card's own lookup.
    vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
      timeZone: "America/New_York",
    } as Intl.ResolvedDateTimeFormatOptions);
    stubRoutes({ "GET /data-subject-requests": () => jsonResponse(DSRS) });
    render(<RequestsPanel />);
    await screen.findByText(/8f3a-contact-uuid/);
    // This codebase's locked locale convention (format.ts's INTL_LOCALE,
    // "A100: unconfigured English is en-GB, not en-US") renders numeric
    // dates DD/MM/YYYY: New York renders 31/07/2026; a hardcoded
    // Europe/Berlin would instead render 01/08/2026 — pin the one this
    // code actually emits, not every format it never does.
    expect(screen.getByText(/31\/07\/2026/)).toBeInTheDocument();
  });

  it("offers only the transitions the server would accept", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    expect(
      within(row).getByRole("button", { name: /in progress/i }),
    ).toBeInTheDocument();
    expect(
      within(row).getByRole("button", { name: /fulfil/i }),
    ).toBeInTheDocument();
  });

  it("offers no transition on a closed request — a closed request never reopens", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /anna@acme.test/i }),
    );
    const row = await findDsrRow("anna@acme.test");
    expect(
      within(row).queryByRole("button", { name: /in progress/i }),
    ).not.toBeInTheDocument();
    // "in progress" alone would still pass if this row offered Fulfil or
    // Reject — nextStatuses(open|in_progress) is the DAG under test here,
    // so pin all three targets it could have offered, not just one of them.
    expect(
      within(row).queryByRole("button", { name: /^fulfil$/i }),
    ).not.toBeInTheDocument();
    expect(
      within(row).queryByRole("button", { name: /^reject$/i }),
    ).not.toBeInTheDocument();
    expect(within(row).getByText(/closed/i)).toBeInTheDocument();
  });

  it("holds a close until its answer and identity check are given — the server 422s without either", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    expect(within(row).getByRole("button", { name: /reject/i })).toBeDisabled();
    await userEvent.type(
      screen.getByLabelText(/resolution/i),
      "not a data subject",
    );
    expect(within(row).getByRole("button", { name: /reject/i })).toBeDisabled();
    await choose(
      screen.getByLabelText(en["privacy.identityCheck"]),
      en["privacy.identityCheck.not_verified"],
    );
    expect(within(row).getByRole("button", { name: /reject/i })).toBeEnabled();
  });

  it("flags an overdue request against the injected clock", async () => {
    // Both fixture rows are before "now"; d2 (anna@acme.test) is fulfilled
    // and isTerminal short-circuits isOverdue regardless of its due date, so
    // a bare page-wide query could pass even if this pinned it to the wrong
    // row (or to none, if it matched the facet bar instead). Scope to the
    // one row this clock genuinely makes overdue: d1, still open.
    vi.setSystemTime(new Date("2026-08-02T00:00:00Z"));
    stubRoutes();
    render(<RequestsPanel />);
    const row = await findDsrRow("8f3a-contact-uuid");
    expect(within(row).getByText(/overdue/i)).toBeInTheDocument();
    const closedRow = await findDsrRow("anna@acme.test");
    expect(within(closedRow).queryByText(/overdue/i)).not.toBeInTheDocument();
  });

  // The stale-row race: another admin moved it first, so our offered
  // transition is now illegal and the PATCH 422s. Note this is NOT the
  // approvals' 409 already_decided — isAlreadyDecided does not apply. The
  // stub mirrors the real wire shape (consent/dsr.go's UpdateDSR via
  // writeConsentErr → httperr.Validation("status", "invalid", reason)):
  // top-level code is always "validation_error", and the field that failed
  // rides in details.errors[0].field — NOT in the top-level code, which the
  // create-purpose test above shows is reused for every validation failure.
  it("re-reads and explains when the request moved on underneath us", async () => {
    stubRoutes({
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail: "open → fulfilled is not a legal transition",
            status: 422,
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "status",
                  code: "invalid",
                  message: "open → fulfilled is not a legal transition",
                },
              ],
            },
          },
          422,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    await draftClose("done");
    await userEvent.click(within(row).getByRole("button", { name: /reject/i }));
    expect(await screen.findByText(/else decided this/i)).toBeInTheDocument();
  });

  // A patch failure that is NOT the illegal-transition 422 must
  // never wear the "moved on" copy — that would tell the officer a colleague
  // made a decision that never happened. A 403 (code "permission_denied") gets
  // the catalog's refusal sentence instead, and never the server's own detail:
  // every producer of that sentinel wraps it in internals — the RBAC object and
  // verb here — which name nothing the reader can act on.
  it("tells the truth about a non-transition patch failure instead of claiming it moved on", async () => {
    stubRoutes({
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse(
          {
            title: "Forbidden",
            // What auth.Require actually sends: the RBAC object and verb
            // wrapped around the sentinel. Internals, not copy — so the screen
            // must show the catalog sentence instead of echoing it.
            detail: "data_subject_request.update: permission denied",
            status: 403,
            code: "permission_denied",
          },
          403,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    await draftClose("done");
    await userEvent.click(within(row).getByRole("button", { name: /reject/i }));
    expect(
      await screen.findByText(en["common.permissionDenied"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(/data_subject_request/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/else decided this/i)).not.toBeInTheDocument();
  });

  it("assigns from the roster", async () => {
    const sent = stubRoutes({
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u1",
              email: "dpo@acme.test",
              display_name: "Dana DPO",
              status: "active",
              is_agent: false,
            },
            {
              id: "u2",
              email: "bot@acme.test",
              display_name: "Bot",
              status: "active",
              is_agent: true,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse({ ...DSRS.data[0], assignee_id: "u1" }),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const picker = await screen.findByLabelText(/assignee/i);
    // The agent seat's absence is a property of the LIST, and the list only
    // exists while the popup is open — so the picker is opened here and the pick
    // lands on the very list under assertion (which is why this drives the two
    // steps by hand rather than through pickOption).
    await userEvent.click(picker);
    const roster = screen.getByRole("listbox");
    expect(
      within(roster).queryByRole("option", { name: "Bot" }),
    ).not.toBeInTheDocument();
    await userEvent.click(
      within(roster).getByRole("option", { name: "Dana DPO" }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "PATCH /data-subject-requests/d1"),
      ).toHaveLength(1),
    );
    // The load-bearing invariant: assignee_id: "u1" actually went on the
    // wire, and NOTHING ELSE did — not even an explicit null for status or
    // resolution. The server's UPDATE sets `coalesce($n, col)` for every
    // field, so a stray `status: null` or `resolution: null` in this body
    // would silently no-op those columns rather than leave them untouched;
    // toEqual (not objectContaining) proves no such key rode along.
    const patches = sent.filter(
      (s) => s.key === "PATCH /data-subject-requests/d1",
    );
    expect(patches[0]?.body).toEqual({ assignee_id: "u1" });
  });

  // An explicit null hands the request back to the unassigned pool.
  it("hands an assigned request back to nobody with an explicit null", async () => {
    const sent = stubRoutes({
      "GET /data-subject-requests": () =>
        jsonResponse({
          ...DSRS,
          data: [{ ...DSRS.data[0], assignee_id: "u1" }, DSRS.data[1]],
        }),
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u1",
              email: "dpo@acme.test",
              display_name: "Dana DPO",
              status: "active",
              is_agent: false,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await choose(await screen.findByLabelText(/assignee/i), "Unassigned");
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "PATCH /data-subject-requests/d1")?.body,
      ).toEqual({ assignee_id: null }),
    );
  });

  // The assignee select and the row's own status-transition buttons share
  // one `patch` mutation, so a failed assignment must be exactly as visible
  // as a failed transition — this pins that it is, using the assignee path
  // specifically (not the transition path other tests already cover).
  it("renders an honest error when an assignee PATCH fails", async () => {
    stubRoutes({
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u1",
              email: "dpo@acme.test",
              display_name: "Dana DPO",
              status: "active",
              is_agent: false,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse(
          {
            title: "Forbidden",
            // What auth.Require actually sends: the RBAC object and verb
            // wrapped around the sentinel. Internals, not copy — so the screen
            // must show the catalog sentence instead of echoing it.
            detail: "data_subject_request.update: permission denied",
            status: 403,
            code: "permission_denied",
          },
          403,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    const picker = await screen.findByLabelText(/assignee/i);
    await choose(picker, "Dana DPO");
    const row = await findDsrRow("8f3a-contact-uuid");
    expect(
      await within(row).findByText(en["common.permissionDenied"]),
    ).toBeInTheDocument();
    expect(
      within(row).queryByText(/data_subject_request/i),
    ).not.toBeInTheDocument();
  });

  // The refusal reaching the reader is the catalog's, whatever the body carried:
  // a bare sentinel here, a sentinel wrapped in the RBAC object and verb in the
  // two tests above. Both land on the same sentence, because it is the only one
  // that says who can widen the access — there is no case where the server's own
  // detail is the words to keep.
  it("renders a scoped rep's 403 honestly", async () => {
    stubRoutes({
      "GET /data-subject-requests": () =>
        jsonResponse(
          {
            title: "permission denied",
            status: 403,
            code: "permission_denied",
          },
          403,
        ),
    });
    render(<RequestsPanel />);
    expect(
      await screen.findByText(en["common.permissionDenied"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(/^permission denied$/i)).not.toBeInTheDocument();
  });
});

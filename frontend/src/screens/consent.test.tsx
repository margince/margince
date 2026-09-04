/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ConsentSection } from "./consent";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// A full seat holding person.update — the default these tests run under.
const SEAT_MAY_WRITE = {
  user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
  authorization: {
    seat_type: "full",
    objects: { person: { read: true, update: true } },
  },
};

// The row the section is asked about. `writable` is what the server answers per
// caller per row, and the section fails closed without it — a fixture that
// omits it describes a contact this reader may not edit, which is a different
// test than the one each of these means to be.
const writablePerson = { writable: true };

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
    {
      id: "p2",
      key: "marketing_email",
      label: "Marketing",
      requires_double_opt_in: true,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

const CONSENT = {
  state: [
    {
      purpose_id: "p1",
      purpose_key: "transactional",
      state: "granted",
      lawful_basis: "Art. 6(1)(b)",
      updated_at: "2026-05-01T10:00:00Z",
    },
    { purpose_id: "p2", purpose_key: "marketing_email", state: "unknown" },
  ],
  events: [
    {
      id: "e1",
      purpose_id: "p1",
      new_state: "granted",
      source: "booking form",
      // Deliberately NOT the current state's basis. If the log row echoed the
      // head row instead of reading its own event, a shared value would still
      // count two and this test would pass over the bug it exists to catch.
      lawful_basis: "Art. 6(1)(f)",
      actor_type: "human",
      actor_id: "u1",
      occurred_at: "2026-05-01T10:00:00Z",
    },
  ],
};

// Records every request so a test can assert what actually went to the
// server — the request body IS the contract for a consent write.
type Sent = { key: string; url: string; body: unknown };

function stubRoutes(
  overrides: Record<string, () => Response> = {},
  sent: Sent[] = [],
) {
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
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, url: url.pathname + url.search, body });
      const override = overrides[key];
      if (override) return override();
      // A full seat holding person.update, which every one of these tests
      // assumed before the grant was asked for: they are about what the
      // section SENDS, and a reader who may not write it sends nothing. The
      // permission axis has its own tests below.
      if (key === "GET /me") return jsonResponse(SEAT_MAY_WRITE);
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /people/person-1/consent") return jsonResponse(CONSENT);
      return jsonResponse({});
    }),
  );
  return sent;
}

// Both fixture purposes render a row with identically-named controls
// ("Proof log", "Grant"), so tests that need one specific row's control
// scope the query to that row rather than assuming which one a bare
// findByRole/getByRole call lands on.
async function findConsentRow(label: string) {
  const row = (await screen.findByText(label)).closest(".consent-row");
  if (!(row instanceof HTMLElement)) {
    throw new Error(`consent row for "${label}" not found`);
  }
  return row;
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ConsentSection", () => {
  it("renders unknown distinctly from withdrawn — no record is not a withdrawal", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    expect(await screen.findByText(/no record/i)).toBeInTheDocument();
  });

  // G-4: the events[] the Person 360 currently drops. Art. 7 demonstrability.
  it("shows the append-only proof log for a purpose", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    expect(await screen.findByText(/booking form/i)).toBeInTheDocument();
  });

  // "When, and on what basis" is the whole of what a subject request, an audit
  // or a handover asks about a consent record. `lawful_basis` was on the wire
  // on both the state and every proof row, and reached no screen: a reader
  // could see that a purpose was granted in May and not what it was granted
  // ON, which is the half that decides whether the grant still stands.
  it("says what basis the current state stands on", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    expect(within(row).getByText(/Art\. 6\(1\)\(b\)/)).toBeInTheDocument();
  });

  it("says what basis each recorded decision was argued from", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    // The two bases are different on purpose, so this names which text came
    // from where: the head row still argues from the state's basis, and the
    // log carries the event's own.
    expect(await screen.findByText(/Art\. 6\(1\)\(f\)/)).toBeInTheDocument();
    expect(within(row).getByText(/Art\. 6\(1\)\(b\)/)).toBeInTheDocument();
  });

  // A record with no basis says nothing rather than claiming one nobody
  // entered. The field is operator-authored free text, so absent is ordinary.
  it("claims no basis for a record that carries none", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Marketing");
    expect(within(row).queryByText(/Basis:/)).toBeNull();
  });

  // C3: the log's actor line must name the ACTUAL actor, never resolve to a
  // claim about the viewer — this fixture's event was captured by u1, a
  // human who is not necessarily whoever is looking at this proof.
  it("names the actual human actor rather than claiming the viewer typed it", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    expect(await screen.findByText("u1")).toBeInTheDocument();
    expect(screen.queryByText(/typed by you/i)).not.toBeInTheDocument();
  });

  // The second leg of the same defect: an event that omits actor_type
  // entirely must not default into the human branch either — an actor the
  // wire never named is unknown, never a positive claim about the viewer.
  it("does not default a missing actor_type to a claim about the viewer", async () => {
    stubRoutes({
      "GET /people/person-1/consent": () =>
        jsonResponse({
          state: CONSENT.state,
          events: [
            {
              id: "e2",
              purpose_id: "p1",
              new_state: "granted",
              source: "import",
              occurred_at: "2026-05-01T10:00:00Z",
            },
          ],
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    expect(await screen.findByText(/actor not recorded/i)).toBeInTheDocument();
    expect(screen.queryByText(/typed by you/i)).not.toBeInTheDocument();
  });

  // A purpose with no consent record has no events by construction — the
  // log must still be reachable and say so honestly, not hide the toggle.
  it("shows the honest empty state for a purpose with no consent record", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Marketing");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    expect(
      await screen.findByText(/no consent decision recorded/i),
    ).toBeInTheDocument();
  });

  // A double opt-in is completed by the SUBJECT, from a link mailed to their
  // own address. This surface offers no way to type one in, because a token an
  // operator can type is a confirmation an operator can forge.
  it("offers no token field on a purpose that requires double opt-in", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await screen.findByText("Marketing");
    expect(screen.queryByLabelText(/confirmation token/i)).toBeNull();
    expect(
      screen.queryByRole("button", { name: /issue double opt-in/i }),
    ).toBeNull();
  });

  // A button that can only 422 is worse than no button: it tells a rep the
  // action exists. Withdraw stays, because taking consent back is always the
  // subject's right and never needs a round trip.
  it("offers no Grant on a double-opt-in row nobody here can grant", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await screen.findByText("Marketing");
    // The fixture holds a granted non-DOI purpose and an unknown DOI one. The
    // granted row keeps Withdraw; the DOI row offers nothing, so no Grant
    // button survives anywhere on the section.
    expect(screen.queryByRole("button", { name: /^grant$/i })).toBeNull();
    expect(screen.getAllByRole("button", { name: /^withdraw$/i })).toHaveLength(
      1,
    );
  });

  it("says who confirms a double-opt-in purpose, on that row only", async () => {
    stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await screen.findByText("Marketing");
    // One row requires DOI in the fixture; the note belongs to it alone.
    expect(
      screen.getAllByText(/confirmed by the contact themselves/i),
    ).toHaveLength(1);
  });

  // Asserted through the withdraw verb on the NON-DOI row, which is the live
  // state-writing button this fixture offers. The point is the request body:
  // no token key reaches the server from this screen any more.
  it("never sends a token with a state write", async () => {
    const sent = stubRoutes({
      "POST /people/person-1/consent": () =>
        jsonResponse({
          purpose_id: "p1",
          purpose_key: "transactional",
          state: "withdrawn",
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await screen.findByText("Marketing");
    await userEvent.click(
      screen.getAllByRole("button", { name: /^withdraw$/i })[0],
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /people/person-1/consent"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /people/person-1/consent");
    expect(posts.at(-1)?.body).not.toHaveProperty("double_opt_in_token");
  });

  it("omits the token key entirely when none was typed", async () => {
    const sent = stubRoutes({
      "POST /people/person-1/consent": () =>
        jsonResponse({
          purpose_id: "p1",
          purpose_key: "transactional",
          state: "withdrawn",
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await userEvent.click(
      await screen.findByRole("button", { name: /^withdraw$/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /people/person-1/consent"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /people/person-1/consent");
    // An empty-string token must not be sent — the server would reject it as
    // "not a currently issued double opt-in token" rather than treat it as absent.
    expect(posts.at(-1)?.body).toEqual({
      purpose_id: "p1",
      new_state: "withdrawn",
    });
  });

  // Preserves people.test.tsx's former "shows Grant for a non-granted purpose"
  // coverage: a plain (non-DOI) grant must send no token key either, and this
  // exercises the ternary badge's `granted` branch for what the fixture's p1
  // otherwise never reaches (it starts already granted).
  it("sends a plain grant with no token key for a purpose that does not require one", async () => {
    const sent = stubRoutes({
      "GET /people/person-1/consent": () =>
        jsonResponse({
          state: [
            {
              purpose_id: "p1",
              purpose_key: "transactional",
              state: "withdrawn",
            },
            {
              purpose_id: "p2",
              purpose_key: "marketing_email",
              state: "unknown",
            },
          ],
          events: [],
        }),
      "POST /people/person-1/consent": () =>
        jsonResponse({
          purpose_id: "p1",
          purpose_key: "transactional",
          state: "granted",
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    // Both p1 (withdrawn) and p2 (unknown) show a Grant button here; [0] is
    // p1's — rows render in the order GET /people/{id}/consent lists them.
    await screen.findByText("Deal messages");
    await userEvent.click(
      screen.getAllByRole("button", { name: /^grant$/i })[0],
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /people/person-1/consent"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /people/person-1/consent");
    expect(posts.at(-1)?.body).toEqual({
      purpose_id: "p1",
      new_state: "granted",
    });
  });

  // Defect-1 regression guard: the write endpoint's response can't carry the
  // new consent_event, so the proof log can only pick up the transition just
  // made by re-reading GET /people/{id}/consent. Proves the refetch actually
  // happens (not just that the badge flips) by having the second GET return
  // an event the first GET never had, then asserting it renders.
  it("re-reads the consent GET after a write so the proof log includes the new decision", async () => {
    let getCalls = 0;
    const sent = stubRoutes({
      "GET /people/person-1/consent": () => {
        getCalls += 1;
        if (getCalls === 1) return jsonResponse(CONSENT);
        return jsonResponse({
          state: [
            { ...CONSENT.state[0], state: "withdrawn" },
            CONSENT.state[1],
          ],
          events: [
            ...CONSENT.events,
            {
              id: "e2",
              purpose_id: "p1",
              new_state: "withdrawn",
              source: "person 360",
              actor_type: "human",
              actor_id: "u1",
              occurred_at: "2026-06-01T00:00:00Z",
            },
          ],
        });
      },
      "POST /people/person-1/consent": () =>
        jsonResponse({
          purpose_id: "p1",
          purpose_key: "transactional",
          state: "withdrawn",
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    const row = await findConsentRow("Deal messages");
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
    expect(await screen.findByText(/booking form/i)).toBeInTheDocument();

    await userEvent.click(
      within(row).getByRole("button", { name: /^withdraw$/i }),
    );

    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "GET /people/person-1/consent"),
      ).toHaveLength(2),
    );
    expect(await screen.findByText(/person 360/i)).toBeInTheDocument();
  });

  // This surface used to mint a DOI token and print it on screen, next to a
  // field for typing it back in. Both halves are gone: nothing here calls the
  // issuance endpoint, so there is no capability for an operator to read.
  it("never asks the server to issue a double-opt-in token", async () => {
    const sent = stubRoutes();
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    await screen.findByText("Marketing");
    await userEvent.click(
      screen.getAllByRole("button", { name: /^withdraw$/i })[0],
    );
    await waitFor(() =>
      expect(sent.some((s) => s.key === "POST /people/person-1/consent")).toBe(
        true,
      ),
    );
    expect(sent.some((s) => s.key.includes("double-opt-in"))).toBe(false);
  });

  it("renders an honest empty state when the workspace tracks no purposes", async () => {
    stubRoutes({
      "GET /consent-purposes": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /people/person-1/consent": () =>
        jsonResponse({ state: [], events: [] }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    expect(await screen.findByText(/no consent purposes/i)).toBeInTheDocument();
  });

  it("surfaces a load failure with a retry rather than a blank card", async () => {
    stubRoutes({
      "GET /people/person-1/consent": () =>
        jsonResponse({ title: "boom", status: 500 }, 500),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    expect(
      await screen.findByRole("button", { name: /retry/i }),
    ).toBeInTheDocument();
  });

  // I6: requires_double_opt_in lives only on ConsentPurpose, so a failed
  // purposes fetch must not fall back to rendering every row as freely
  // grantable — share.tsx's RosterPicker gates its two roster fetches the
  // same explicit way, for the same reason (a collapsed-to-[] failure must
  // never be mistaken for a real empty list).
  it("shows an error instead of quietly dropping the DOI gate when purposes fail to load", async () => {
    stubRoutes({
      "GET /consent-purposes": () => jsonResponse({ title: "boom" }, 500),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);
    expect(
      await screen.findByText(/couldn't load the consent purpose catalogue/i),
    ).toBeInTheDocument();
    // The DOI-required "Marketing" row must not render as freely grantable
    // with no sign anything failed.
    expect(screen.queryByText("Marketing")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /issue double opt-in/i }),
    ).not.toBeInTheDocument();
  });
});

// The link the confirm page has always been able to render, and that nothing
// could send until now. What these hold is the difference between a link that
// went out, a link that exists and reached nobody, and a caller aiming it.
describe("asking a contact to confirm their details", () => {
  const MAY_WRITE = {
    user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
    authorization: {
      seat_type: "full",
      objects: { person: { read: true, update: true } },
    },
  };

  it("reports the address it went to, which the caller never chose", async () => {
    const sent: Sent[] = [];
    stubRoutes(
      {
        "GET /me": () => jsonResponse(MAY_WRITE),
        "POST /people/person-1/consent/confirm-request": () =>
          jsonResponse(
            {
              delivered_to: "ada@example.test",
              expires_at: "2026-09-13T09:00:00Z",
              queued: true,
              sendable: true,
            },
            201,
          ),
      },
      sent,
    );
    render(<ConsentSection personId="person-1" person={writablePerson} />);

    await userEvent.click(await screen.findByTestId("confirm-details-ask"));

    expect(await screen.findByTestId("confirm-details-sent")).toHaveTextContent(
      /ada@example\.test/,
    );
    // The request carries no address and no token: the server derives one and
    // never hands back the other, which is what lets the answer stand as the
    // subject's own.
    const ask = sent.find((one) => one.key.includes("confirm-request"));
    expect(ask?.body).toBeFalsy();
  });

  it("says a queued link is on its way rather than claiming it was delivered", async () => {
    stubRoutes({
      "GET /me": () => jsonResponse(MAY_WRITE),
      "POST /people/person-1/consent/confirm-request": () =>
        jsonResponse(
          {
            delivered_to: "ada@example.test",
            expires_at: "2026-09-13T09:00:00Z",
            queued: true,
            sendable: true,
          },
          201,
        ),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);

    await userEvent.click(await screen.findByTestId("confirm-details-ask"));

    // The message is staged here and transmitted later by the dispatcher, which
    // retries and can park. "Sent" would tell a rep somebody was asked while the
    // message is still waiting, and they would stop watching for an answer.
    const sent = await screen.findByTestId("confirm-details-sent");
    expect(sent).toHaveTextContent(/on its way/i);
    expect(sent).not.toHaveTextContent(/sends no mail/i);
  });

  it("says plainly when the link exists but nobody was sent it", async () => {
    stubRoutes({
      "GET /me": () => jsonResponse(MAY_WRITE),
      "POST /people/person-1/consent/confirm-request": () =>
        jsonResponse(
          {
            delivered_to: "ada@example.test",
            expires_at: "2026-09-13T09:00:00Z",
            queued: false,
          },
          201,
        ),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);

    await userEvent.click(await screen.findByTestId("confirm-details-ask"));

    // A rep who reads "sent" here would believe a contact was asked when they
    // never were, and would stop waiting for an answer that cannot come.
    expect(await screen.findByTestId("confirm-details-sent")).toHaveTextContent(
      /sends no mail/i,
    );
  });

  it("shows a contact with no address as a refusal", async () => {
    stubRoutes({
      "GET /me": () => jsonResponse(MAY_WRITE),
      "POST /people/person-1/consent/confirm-request": () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail:
              "this contact carries no live email address, so there is no mailbox a confirm link could reach",
            status: 422,
          },
          422,
        ),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);

    await userEvent.click(await screen.findByTestId("confirm-details-ask"));

    expect(await screen.findByText(/no live email address/i)).toBeVisible();
    expect(
      screen.queryByTestId("confirm-details-sent"),
    ).not.toBeInTheDocument();
  });

  it("is not offered to a caller who may not write the person", async () => {
    stubRoutes({
      "GET /me": () =>
        jsonResponse({
          user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
          authorization: {
            seat_type: "read",
            objects: { person: { read: true } },
          },
        }),
    });
    render(<ConsentSection personId="person-1" person={writablePerson} />);

    // Waited for rather than asserted immediately: the capability arrives
    // asynchronously, so an absence checked too early passes for the wrong
    // reason.
    expect(await screen.findByText(/no record/i)).toBeInTheDocument();
    expect(screen.queryByTestId("confirm-details-ask")).not.toBeInTheDocument();
  });
});

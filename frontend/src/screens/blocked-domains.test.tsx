// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { GrantSpec } from "../testing/mefixture";
import { openRowMenu } from "../testing/tablerow.testkit";
import { BlockedDomainsCard } from "./blocked-domains";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "./story-utils";

// Settings → Capture: the domains this installation refuses a company. The card
// answers one question nothing else can — a company that never appeared, was it
// refused and by whom — so what it must never do is lose the difference between
// a machine's refusal and a contact's, claim an empty list is a broken one, or
// offer a write it has no reason to send.
//
// The write is three inputs submitted together, so it lives in a dialog behind
// one verb: a test that used to type into the card now opens the dialog first,
// and what LANDED is still reported on the card, because the dialog is gone by
// the time it is true.

const LIST = "GET /capture/blocked-domains";
const WRITE = "PUT /capture/blocked-domains";
const REOPEN = "POST /capture/blocked-domains/pwc.example/reopen";

// Read is every human role's; changing an entry is company:update.
const OPS: GrantSpec = { company: ["read", "update"] };
const READER: GrantSpec = { company: ["read"] };

// `reason` is the deprecated English sentence; the card renders `reason_code`.
const BY_HEURISTIC = {
  domain: "expensify.example",
  admission: "suppressed",
  reason: "bulk sender: no reply address",
  reason_code: "bulk_sender",
  sender_kind: "transactional",
  source: "heuristic",
  decided_at: "2026-08-02T14:40:00Z",
  company_id: null,
};
const BY_HUMAN = {
  domain: "mckinsey.example",
  admission: "admitted",
  reason: "they became a client in July",
  reason_code: "stated",
  note: "they became a client in July",
  source: "human",
  decided_at: "2026-08-11T07:05:00Z",
  company_id: "018f3a1b-0000-7000-8000-00000000c001",
  company_name: "McKinsey and Company",
};

// A domain nothing decided: the machine gave up, cleared its retry cursor, and
// nothing will ask again on its own.
const UNDECIDED = {
  domain: "pwc.example",
  admission: "undecided",
  // Distinct from the source LABEL beside it on purpose: the two cells say
  // different things, and a fixture whose reason repeats its source makes an
  // assertion about either one ambiguous.
  reason:
    "Nothing on the site named a company, and the sender's name did not explain the domain.",
  reason_code: "unevidenced",
  source: "unevidenced",
  decided_at: "2026-08-14T11:20:00Z",
  company_id: null,
};

function mount(allow: GrantSpec, routes: RouteMap) {
  installFetchStub({ "GET /me": meRoute(allow), ...routes });
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BlockedDomainsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/** The dialog the card's one write verb opens, with the form inside it. */
async function openDecisionDialog(user: UserEvent): Promise<HTMLElement> {
  await user.click(
    screen.getByRole("button", { name: en["blockedDomains.recordOpen"] }),
  );
  return screen.getByRole("dialog");
}

/** The row a domain is on, so an assertion is about that domain's own cells. */
function rowFor(domain: string): HTMLElement {
  const cell = screen.getByText(domain);
  const row = cell.closest("tr");
  if (row === null) {
    throw new Error(`${domain} is not in a table row`);
  }
  return row;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("BlockedDomainsCard", () => {
  it("says of every decision whether a machine or a contact made it", async () => {
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [BY_HEURISTIC, BY_HUMAN], total: 2 }),
    });

    await screen.findByText("expensify.example");
    // The distinction the card exists for: the same outcome, two different
    // facts. A row that named only the admission would leave an operator
    // unable to tell a bulk-sender verdict from somebody's deliberate call.
    expect(
      within(rowFor("expensify.example")).getByText(
        en["blockedDomains.source.heuristic"],
      ),
    ).toBeInTheDocument();
    expect(
      within(rowFor("mckinsey.example")).getByText(
        en["blockedDomains.source.human"],
      ),
    ).toBeInTheDocument();
    // And the reason, in the reader's words rather than the server's English.
    expect(
      within(rowFor("expensify.example")).getByText(
        en["blockedDomains.reason.bulkTransactional"],
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("bulk sender: no reply address")).toBeNull();
    // A human's reason is their own sentence.
    expect(
      within(rowFor("mckinsey.example")).getByText(
        "they became a client in July",
      ),
    ).toBeInTheDocument();
  });

  it("names the company a domain produced and opens it", async () => {
    const user = userEvent.setup();
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [BY_HUMAN], total: 1 }),
    });

    await user.click(
      await screen.findByRole("button", { name: "McKinsey and Company" }),
    );
    expect(globalThis.location.hash).toContain(
      "/companies/018f3a1b-0000-7000-8000-00000000c001",
    );
  });

  it("reads as nothing-refused rather than as a table that failed to draw", async () => {
    mount(OPS, { [LIST]: () => jsonResponse({ data: [], total: 0 }) });

    expect(await screen.findByText(en["blockedDomains.none"])).toBeVisible();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("says how many decisions exist, not only how many this page carries", async () => {
    // Refusals accumulate on their own from every bulk-sender verdict, so the
    // server pages the list. An operator hunting a company that never appeared
    // must be able to tell "not refused" from "past the end of this page".
    mount(OPS, {
      [LIST]: () =>
        jsonResponse({ data: [BY_HEURISTIC, BY_HUMAN], total: 412 }),
    });

    await screen.findByText("expensify.example");
    expect(screen.getByText(/412/)).toBeInTheDocument();
  });

  it("sends the domain, the decision and the reason a human typed", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [], total: 0 }),
      [WRITE]: (body) => {
        sent.push(body);
        return jsonResponse({
          ...BY_HEURISTIC,
          domain: "expensify.example",
          source: "human",
          reason: "Supplier, not a customer",
        });
      },
    });

    await screen.findByText(en["blockedDomains.none"]);
    const dialog = await openDecisionDialog(user);
    await user.type(
      within(dialog).getByTestId("blocked-domain-input"),
      "mail.expensify.example",
    );
    await user.type(
      within(dialog).getByTestId("blocked-domain-reason"),
      "Supplier, not a customer",
    );
    await user.click(
      within(dialog).getByRole("button", { name: en["blockedDomains.save"] }),
    );

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      domain: "mail.expensify.example",
      admission: "suppressed",
      reason: "Supplier, not a customer",
    });
    // The server normalizes the domain and the write REPLACES any decision
    // already on it, so what landed is named: without it a sub-domain silently
    // becomes its parent, and a second decision on a listed domain looks like
    // nothing happened.
    expect(await screen.findByRole("status")).toHaveTextContent(
      "expensify.example",
    );
  });

  it("refuses the write until there is a reason somebody can review", async () => {
    const user = userEvent.setup();
    mount(OPS, { [LIST]: () => jsonResponse({ data: [], total: 0 }) });

    await screen.findByText(en["blockedDomains.none"]);
    const dialog = await openDecisionDialog(user);
    const save = within(dialog).getByRole("button", {
      name: en["blockedDomains.save"],
    });
    await user.type(
      within(dialog).getByTestId("blocked-domain-input"),
      "expensify.example",
    );
    // A domain with no reason is exactly what the server 422s, and a refusal
    // nobody can explain is one nobody can review — so the card never sends it.
    expect(save).toBeDisabled();
    await user.type(
      within(dialog).getByTestId("blocked-domain-reason"),
      "a vendor",
    );
    expect(save).toBeEnabled();
  });

  it("says why the write was refused rather than leaving the form silent", async () => {
    const user = userEvent.setup();
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [], total: 0 }),
      [WRITE]: () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail:
              "expected a domain name like example.com; a full email address or a URL is not one",
          },
          422,
        ),
    });

    await screen.findByText(en["blockedDomains.none"]);
    const dialog = await openDecisionDialog(user);
    await user.type(
      within(dialog).getByTestId("blocked-domain-input"),
      "someone@expensify.example",
    );
    await user.type(
      within(dialog).getByTestId("blocked-domain-reason"),
      "a vendor",
    );
    await user.click(
      within(dialog).getByRole("button", { name: en["blockedDomains.save"] }),
    );

    // Beside the form that was refused, and the form is still standing: a
    // dialog that closed on a 422 would take the typing with it.
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /a full email address or a URL is not one/,
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("hands a row's flip to the dialog, because a reversal still owes a reason", async () => {
    const user = userEvent.setup();
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [BY_HUMAN], total: 1 }),
    });

    await screen.findByText("mckinsey.example");
    // The row's verb is the OPPOSITE of the standing decision, and taking it
    // opens the form rather than writing: the contract demands a reason and a
    // one-click flip has nowhere to get one.
    const menu = await openRowMenu(user, "mckinsey.example");
    expect(
      menu.queryByRole("button", { name: en["blockedDomains.rowAdmit"] }),
    ).toBeNull();
    await user.click(
      menu.getByRole("button", { name: en["blockedDomains.rowRefuse"] }),
    );

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByTestId("blocked-domain-input")).toHaveValue(
      "mckinsey.example",
    );
    // The decision is already flipped, and the reason is EMPTY: the sentence
    // that justified admitting this domain cannot stand as the one that
    // refuses it.
    expect(
      within(dialog).getByRole("combobox", {
        name: en["blockedDomains.admissionLabel"],
      }),
    ).toHaveTextContent(en["blockedDomains.admission.suppressed"]);
    expect(within(dialog).getByTestId("blocked-domain-reason")).toHaveValue("");
  });

  it("offers an undecided domain a re-ask rather than a decision it has no grounds for", async () => {
    const user = userEvent.setup();
    const asked: string[] = [];
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [UNDECIDED, BY_HUMAN], total: 2 }),
      [REOPEN]: () => {
        asked.push("pwc.example");
        return jsonResponse(UNDECIDED);
      },
    });

    await screen.findByText("pwc.example");
    const menu = await openRowMenu(user, "pwc.example");
    // Not knowing yet is why the row is listed. A call either way would have
    // no grounds.
    expect(
      menu.queryByRole("button", { name: en["blockedDomains.rowAdmit"] }),
    ).toBeNull();
    expect(
      menu.queryByRole("button", { name: en["blockedDomains.rowRefuse"] }),
    ).toBeNull();

    await user.click(
      menu.getByRole("button", { name: en["blockedDomains.rowReopen"] }),
    );
    await waitFor(() => expect(asked).toEqual(["pwc.example"]));
    // A re-ask changes nothing visible in the row, so the press says it landed.
    expect(await screen.findByRole("status")).toHaveTextContent("pwc.example");
  });

  it("offers a re-ask only where the question is open", async () => {
    const user = userEvent.setup();
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [BY_HUMAN], total: 1 }),
    });

    await screen.findByText("mckinsey.example");
    const menu = await openRowMenu(user, "mckinsey.example");
    // A decided domain answers a re-ask 409.
    expect(
      menu.queryByRole("button", { name: en["blockedDomains.rowReopen"] }),
    ).toBeNull();
  });

  it("says what stopped the machine deciding, not merely that nothing did", async () => {
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [UNDECIDED], total: 1 }),
    });

    await screen.findByText("pwc.example");
    const row = rowFor("pwc.example");
    expect(
      within(row).getByText(en["blockedDomains.admission.undecided"]),
    ).toBeInTheDocument();
    // The reason is what an operator can act on: a site that named nothing is
    // a different problem from mail too old to trust.
    expect(
      within(row).getByText(en["blockedDomains.source.unevidenced"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(UNDECIDED.reason)).toBeNull();
  });

  it("says a colleague kept the domain when they answered its question", async () => {
    mount(OPS, {
      [LIST]: () =>
        jsonResponse({
          data: [
            {
              ...BY_HUMAN,
              reason_code: "kept_as_company",
              note: undefined,
              reason: "kept from the question",
            },
          ],
          total: 1,
        }),
    });

    await screen.findByText("mckinsey.example");
    expect(
      within(rowFor("mckinsey.example")).getByText(
        en["blockedDomains.reason.keptAsCompany"],
      ),
    ).toBeInTheDocument();
  });

  // A reason the server has no code for keeps the sentence it stored.
  it("shows the stored sentence for a reason the server cannot name", async () => {
    mount(OPS, {
      [LIST]: () =>
        jsonResponse({
          data: [{ ...UNDECIDED, reason_code: "unknown" }],
          total: 1,
        }),
    });

    await screen.findByText("pwc.example");
    expect(
      within(rowFor("pwc.example")).getByText(UNDECIDED.reason),
    ).toBeInTheDocument();
  });

  // A server older than `reason_code` still sends its sentence.
  it("shows the stored sentence when the server sends no reason code", async () => {
    const { reason_code: _absent, ...older } = UNDECIDED;
    mount(OPS, {
      [LIST]: () => jsonResponse({ data: [older], total: 1 }),
    });

    await screen.findByText("pwc.example");
    expect(
      within(rowFor("pwc.example")).getByText(UNDECIDED.reason),
    ).toBeInTheDocument();
  });

  it("keeps the list readable for a seat that may not change it, and says so", async () => {
    const user = userEvent.setup();
    mount(READER, {
      [LIST]: () => jsonResponse({ data: [BY_HUMAN], total: 1 }),
    });

    // Withheld, never absent: hiding the card from a reader who may read the
    // posture would answer "was this domain refused" with silence.
    await screen.findByText("mckinsey.example");
    const record = screen.getByRole("button", {
      name: en["blockedDomains.recordOpen"],
    });
    expect(record).toBeDisabled();
    const denial = screen.getByText(en["blockedDomains.adminOnly"]);
    expect(record.getAttribute("aria-describedby")).toBe(denial.id);
    // The row's own verb is refused by the same fact and points at the same
    // sentence — a per-row flip that stayed live would be a way around it.
    const menu = await openRowMenu(user, "mckinsey.example");
    const flip = menu.getByRole("button", {
      name: en["blockedDomains.rowRefuse"],
    });
    expect(flip).toBeDisabled();
    expect(flip.getAttribute("aria-describedby")).toBe(denial.id);
    // And the form is not on the page at all: there is nothing to type into
    // for a seat that may not write.
    expect(screen.queryByTestId("blocked-domain-input")).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { companyDocumentsKey } from "../../lib/attachmentkeys";
import { ANA, BEN, contactCase } from "../../testing/contactcase.fixtures";
import { PairDecision } from "./worklist.pair";

// Deciding a duplicate where it is shown.
//
// The payload has always been on the wire — the contract says it is sent "so
// the decision can be MADE where it is shown" — and the queue drew the row
// without it. These cases are about the decision actually reaching the server,
// and about the one answer that must never be guessed on the reader's behalf.

type WorklistItem = components["schemas"]["WorklistItem"];

const LEFT = "01a05500-0000-7000-8000-0000000000a1";
const RIGHT = "01a05500-0000-7000-8000-0000000000a2";

function pairRow(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000ff",
    source: "dedupe_candidate",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    // BOTH verbs, which is what the lane sends a reader who may write both
    // records over an ordinary pair. The unmergeable pair below is the case
    // where they come apart.
    actions: ["merge", "dismiss"],
    pair: {
      left: {
        id: LEFT,
        label: "Acme GmbH",
        detail: "acme.de",
        related_count: 12,
      },
      right: {
        id: RIGHT,
        label: "ACME Gmbh",
        detail: "acme.de",
        related_count: 1,
      },
      evidence: [],
    },
    ...over,
  };
}

function draw(
  item: WorklistItem,
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  }),
) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <PairDecision item={item} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function stubOk() {
  const fetched = vi.fn(
    async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(null, { status: 204 }),
  );
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

// The generated client calls fetch with a Request, so the body is on it rather
// than in an init object.
async function bodyOf(fetched: ReturnType<typeof stubOk>) {
  const [input, init] = fetched.mock.calls[0];
  if (input instanceof Request) {
    return JSON.parse(await input.text());
  }
  return JSON.parse(String(init?.body));
}

async function confirmMerge(user: UserEvent) {
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: "Merge",
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("deciding a duplicate pair on the row", () => {
  it("names both records and what hangs off each", () => {
    draw(pairRow());
    expect(screen.getByText("Acme GmbH")).toBeTruthy();
    expect(screen.getByText("ACME Gmbh")).toBeTruthy();
    // The reader's best single signal for which side is the real one.
    expect(screen.getByText("12 linked")).toBeTruthy();
    expect(screen.getByText("1 linked")).toBeTruthy();
  });

  // Each verb NAMES the record it keeps. Two buttons reading alike over an
  // irreversible merge leave a reader who is not looking at the layout no way
  // to tell which record they are about to archive — and a test picking one by
  // index would not notice, which is how the identical pair got this far.
  it("names the record each verb would keep", () => {
    draw(pairRow());
    expect(screen.getByRole("button", { name: "Keep Acme GmbH" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Keep ACME Gmbh" })).toBeTruthy();
  });

  // A reader who may change only one record may only give it away, so only
  // the other record offers Keep.
  it("offers only the record the server says must survive", () => {
    const row = pairRow();
    draw({ ...row, pair: row.pair && { ...row.pair, keep_id: RIGHT } });
    expect(screen.queryByRole("button", { name: "Keep Acme GmbH" })).toBeNull();
    expect(screen.getByRole("button", { name: "Keep ACME Gmbh" })).toBeTruthy();
  });

  // THE case this component exists for. Merging KEEPS one record and archives
  // the other, so the winner must be the one the reader pressed — picking for
  // them would be the product deciding which of a customer's records is real.
  it("merges into the record the reader chose, once they confirm", async () => {
    const user = userEvent.setup();
    const fetched = stubOk();
    draw(pairRow({ kind: "company" }));

    // BY NAME, not by position: this is the assertion that the button the
    // reader believes they pressed is the record that survives.
    await user.click(screen.getByRole("button", { name: "Keep ACME Gmbh" }));
    // A merge cannot be taken back, so the press only asks.
    expect(fetched).not.toHaveBeenCalled();
    await confirmMerge(user);

    await vi.waitFor(() => expect(fetched).toHaveBeenCalled());
    const body = await bodyOf(fetched);
    expect(body.disposition).toBe("merge");
    expect(body.winner_id).toBe(RIGHT);
  });

  it("names the record the merge archives, and says when it is done", async () => {
    const user = userEvent.setup();
    stubOk();
    draw(pairRow({ kind: "company" }));

    await user.click(screen.getByRole("button", { name: "Keep ACME Gmbh" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        "Merge Acme GmbH (acme.de) into ACME Gmbh (acme.de)? The record you keep takes over the history, tasks, tags and links of the other, and fills its empty standard fields from it. The other is archived and keeps its custom field values. A merge cannot be undone.",
      ),
    ).toBeTruthy();
    await confirmMerge(user);

    expect(
      await screen.findByText(
        "Acme GmbH (acme.de) was merged into ACME Gmbh (acme.de).",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("merges nothing when the question is cancelled", async () => {
    const user = userEvent.setup();
    const fetched = stubOk();
    draw(pairRow({ kind: "company" }));

    await user.click(screen.getByRole("button", { name: "Keep ACME Gmbh" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Cancel",
      }),
    );

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fetched).not.toHaveBeenCalled();
  });

  it("says why the two look alike and links each record", () => {
    const row = pairRow({ kind: "company" });
    draw({
      ...row,
      pair: row.pair && {
        ...row.pair,
        evidence: [
          {
            field: "domain",
            signal: "collide",
            left_value: "acme.de",
            right_value: "acme.de",
          },
        ],
      },
    });

    expect(screen.getByText("Domain: acme.de · acme.de")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "ACME Gmbh" }).getAttribute("href"),
    ).toMatch(new RegExp(`#/companies/${RIGHT}$`));
  });

  it("draws no evidence line for a name match, which the card titles already show", () => {
    const row = pairRow({ kind: "company" });
    const { container } = draw({
      ...row,
      pair: row.pair && {
        ...row.pair,
        evidence: [
          {
            field: "display_name",
            signal: "collide",
            left_value: "Acme GmbH",
            right_value: "ACME Gmbh",
          },
        ],
      },
    });

    expect(screen.queryByText(/^Name:/)).toBeNull();
    expect(container.querySelector(".worklist-pair-evidence")).toBeNull();
  });

  it("refreshes both companies' document libraries once the merge lands", async () => {
    const user = userEvent.setup();
    stubOk();
    const client = new QueryClient();
    for (const id of [LEFT, RIGHT]) {
      client.setQueryData(companyDocumentsKey(id), { data: [] });
    }
    draw(pairRow({ kind: "company" }), client);
    await user.click(screen.getByRole("button", { name: "Keep ACME Gmbh" }));
    await confirmMerge(user);
    await vi.waitFor(() =>
      expect(
        [LEFT, RIGHT].map(
          (id) => client.getQueryState(companyDocumentsKey(id))?.isInvalidated,
        ),
      ).toEqual([true, true]),
    );
  });

  // The other answer settles the pair for everybody and needs no winner. The
  // server refuses `winner_id` here, so sending one would 422 a decision the
  // reader made correctly.
  it("sends no winner when the records are not the same", async () => {
    const user = userEvent.setup();
    const fetched = stubOk();
    draw(pairRow());

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));

    await vi.waitFor(() => expect(fetched).toHaveBeenCalled());
    const body = await bodyOf(fetched);
    expect(body.disposition).toBe("not_a_duplicate");
    // No winner reaches the server, which is what its refusal is about. The
    // assertion is on the WIRE rather than on the object the hook built: a key
    // set to undefined and a key never set serialise to the same two bytes, so
    // the distinction the caller makes is only visible here as an absence.
    expect(Object.hasOwn(body, "winner_id")).toBe(false);
    // And the caller is the half that can actually get this wrong: it must not
    // name a survivor for an answer that keeps both records.
    expect(body.winner_id).toBeUndefined();
  });

  // Saying they differ changes neither record, so it is answered at once and
  // offers the one clean way back.
  it("confirms the dismissal and takes it back on Undo", async () => {
    // The Undo toast withdraws itself on a timer; shouldAdvanceTime keeps
    // react-query's own timers running under the fake clock.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetched = stubOk();
    draw(pairRow());

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));
    expect(await screen.findByText("Marked as not duplicates.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await vi.waitFor(() => expect(fetched).toHaveBeenCalledTimes(2));
    const [undone] = fetched.mock.calls[1];
    expect(undone instanceof Request && new URL(undone.url).pathname).toMatch(
      /\/dedupe\/candidates\/.+\/undo$/,
    );
  });

  // A row whose reader may not see both sides arrives without the payload, and
  // the lane already withheld `merge` for that reason. Drawing a decision here
  // would offer a verb over records the client cannot name.
  it("draws nothing when the server withheld the pair", () => {
    const { container } = draw(pairRow({ pair: undefined, actions: [] }));
    expect(container.firstChild).toBeNull();
  });

  // A refused decision leaves the row rendering exactly as an unpressed one
  // does, and the reader believes the pair is settled when it is not.
  //
  // A CONFLICT means the decision did not land as asked — a colleague settled
  // it first, or the records moved underneath. It deliberately does NOT claim
  // the pair is gone: a merge the server refuses REOPENS the candidate, so
  // "already settled" would be false exactly when the reader needs the truth.
  it("says the decision did not land when the server answers conflict", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: "already_disposed" }), {
            status: 409,
            headers: { "content-type": "application/problem+json" },
          }),
      ),
    );
    draw(pairRow());

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));

    expect(
      await screen.findByText(/someone may have decided first/),
    ).toBeTruthy();
    // Never the claim that it is gone: a refused merge puts the pair back.
    expect(screen.queryByText(/gone from the list/)).toBeNull();
  });

  // A REFUSAL will refuse again however often it is pressed, so "try again" is
  // the one instruction that must not appear. This is the toast a rep met on
  // every pair spanning two owners.
  it("names the steward when the decision is refused, and never says try again", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: "permission_denied" }), {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          }),
      ),
    );
    draw(pairRow());

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));

    expect(await screen.findByText(/cannot resolve this pair/)).toBeTruthy();
    expect(screen.queryByText(/Retry\./)).toBeNull();
  });

  // A refusal a press cannot fix has to outlast the press. Both reasoned
  // branches say something the reader must act on somewhere else — hand the
  // pair to an admin, or reload to see where it stands — and a sentence shown
  // for three and a half seconds is one they are invited to miss.
  //
  // Asserted through the DISMISS control, which is what a sticky toast draws
  // and a self-withdrawing one does not. No timer, so this says the same thing
  // on a loaded runner as on an idle laptop.
  it.each([
    { what: "a refusal", code: "permission_denied", status: 403 },
    {
      what: "a pair somebody else settled",
      code: "already_disposed",
      status: 409,
    },
  ])(
    "keeps $what on screen until it is dismissed",
    async ({ code, status }) => {
      const user = userEvent.setup();
      vi.stubGlobal(
        "fetch",
        vi.fn(
          async () =>
            new Response(JSON.stringify({ code }), {
              status,
              headers: { "content-type": "application/problem+json" },
            }),
        ),
      );
      draw(pairRow());

      await user.click(screen.getByRole("button", { name: "Not duplicates" }));

      expect(await screen.findByRole("button", { name: "Close" })).toBeTruthy();
    },
  );

  // And the one that really is retryable still withdraws itself. Without this
  // the case above passes against a toast that never leaves, which would be a
  // different defect wearing the fix's clothes.
  it("lets an unexplained failure withdraw itself", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: "internal" }), {
            status: 500,
            headers: { "content-type": "application/problem+json" },
          }),
      ),
    );
    draw(pairRow());

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));

    expect(await screen.findByText(/Retry\./)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });

  // An identical name is one of the things that MAKES two records look like
  // duplicates, so this is not the rare case. Two buttons over an irreversible
  // merge sharing one accessible name leave a reader who is not looking at the
  // layout unable to tell which record they are about to archive.
  it("tells the two verbs apart when the records share a name", () => {
    draw(
      pairRow({
        pair: {
          left: { id: LEFT, label: "Acme GmbH", detail: "acme.de" },
          right: { id: RIGHT, label: "Acme GmbH", detail: "acme-group.de" },
          evidence: [],
        },
      }),
    );

    // The visible text is unchanged and is still the prefix of each spoken
    // name, so what is seen and what is heard do not come apart.
    expect(
      screen.getByRole("button", { name: "Keep Acme GmbH — acme.de" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Keep Acme GmbH — acme-group.de" }),
    ).toBeTruthy();
  });

  // A company with no domain has no detail line, and two of them with one name
  // are told apart by when each was created.
  it("tells two same-name sides without a detail apart by their age", () => {
    draw(
      pairRow({
        pair: {
          left: {
            id: LEFT,
            label: "Acme GmbH",
            created_at: "2026-03-02T10:00:00Z",
          },
          right: {
            id: RIGHT,
            label: "Acme GmbH",
            created_at: "2026-09-14T10:00:00Z",
          },
          evidence: [],
        },
      }),
    );

    const names = screen
      .getAllByRole("button", { name: /^Keep Acme GmbH — Created / })
      .map((button) => button.getAttribute("aria-label"));
    expect(names).toHaveLength(2);
    expect(new Set(names).size).toBe(2);
  });

  // Two same-name records from one import share a creation day, and often a
  // creation minute too. The names reach further only while they still collide.
  it.each([
    {
      when: "on one day",
      left: "2026-11-02T10:00:00Z",
      right: "2026-11-02T11:00:00Z",
    },
    {
      when: "at one instant",
      left: "2026-11-02T10:00:00Z",
      right: "2026-11-02T10:00:00Z",
    },
  ])("tells two same-name sides created $when apart", ({ left, right }) => {
    draw(
      pairRow({
        pair: {
          left: { id: LEFT, label: "Acme GmbH", created_at: left },
          right: { id: RIGHT, label: "Acme GmbH", created_at: right },
          evidence: [],
        },
      }),
    );

    const names = screen
      .getAllByRole("button", { name: /^Keep Acme GmbH — / })
      .map((button) => button.getAttribute("aria-label"));
    expect(names).toHaveLength(2);
    expect(new Set(names).size).toBe(2);
  });

  // The cards a sighted reader compares must differ where the button names do.
  // The creation minute replaces the day, and the ID joins it at one instant.
  it.each([
    {
      when: "on one day",
      at: ["2026-11-02T10:00:00Z", "2026-11-02T11:00:00Z"],
      ids: [LEFT, RIGHT],
    },
    {
      when: "at one instant",
      at: ["2026-11-02T10:00:00Z", "2026-11-02T10:00:00Z"],
      ids: [LEFT, RIGHT],
    },
    {
      when: "at one instant with IDs ending alike",
      at: ["2026-11-02T10:00:00Z", "2026-11-02T10:00:00Z"],
      ids: [
        "01a05500-0000-7000-8000-1000000000a1",
        "01a05500-0000-7000-8000-2000000000a1",
      ],
    },
  ])(
    "shows what tells two same-name cards created $when apart",
    ({ at, ids }) => {
      draw(
        pairRow({
          pair: {
            left: { id: ids[0], label: "Acme GmbH", created_at: at[0] },
            right: { id: ids[1], label: "Acme GmbH", created_at: at[1] },
            evidence: [],
          },
        }),
      );

      const cards = screen
        .getAllByRole("listitem")
        .filter((card) => card.classList.contains("worklist-pair-card"));
      expect(cards).toHaveLength(2);
      expect(cards[0].textContent).not.toBe(cards[1].textContent);
      const names = screen
        .getAllByRole("button", { name: /^Keep Acme GmbH — / })
        .map((button) => button.getAttribute("aria-label"));
      expect(new Set(names).size).toBe(2);
    },
  );

  // The ordinary pair keeps the short name: the detail is a disambiguator, not
  // decoration, and appending it always would make every button longer to say
  // the same thing.
  it("keeps the plain verb when the names already differ", () => {
    draw(pairRow());
    expect(screen.getByRole("button", { name: "Keep Acme GmbH" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Keep ACME Gmbh" })).toBeTruthy();
  });

  // The pair is still SHOWN — a reader who cannot settle it still needs to know
  // a duplicate is waiting — but nothing is offered, because every press would
  // refuse.
  it("shows the pair without verbs when the server offered nothing", () => {
    draw(pairRow({ actions: [] }));

    expect(screen.getByText("Acme GmbH")).toBeTruthy();
    expect(screen.getByText("ACME Gmbh")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
    expect(
      screen.getByText(/Only a user who can edit both records/),
    ).toBeTruthy();
  });

  // The steward still gets the verbs. Without this the case above would pass
  // against a card that had simply stopped offering them to anybody.
  it("offers the verbs when the server offered merge", () => {
    draw(pairRow());

    expect(screen.getByRole("button", { name: "Keep Acme GmbH" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Not duplicates" })).toBeTruthy();
  });

  // THE PAIR NOBODY CAN MERGE, which is the case the two above cannot see.
  //
  // Two companies each running live projects do not combine, whoever
  // presses — so the lane sends `dismiss` alone. It is a real false positive
  // and the reader is entitled to clear it; guarding this verb on `merge` left
  // the pair on the page as a question nobody could answer here.
  it("offers only the dismissal when the merge is off the table", () => {
    draw(pairRow({ actions: ["dismiss"] }));

    expect(screen.queryByRole("button", { name: "Keep Acme GmbH" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Keep ACME Gmbh" })).toBeNull();
    expect(screen.getByRole("button", { name: "Not duplicates" })).toBeTruthy();
    // And NOT the steward sentence: this reader is the one who can act.
    expect(
      screen.queryByText(/Only a user who can edit both records/),
    ).toBeNull();
  });

  // The lead line is the question the verbs answer, so it changes with them.
  // "Which record should be kept?" over a card with no Keep button asks for an
  // answer the page will not take.
  it("asks whether they are the same at all when neither can survive", () => {
    draw(pairRow({ actions: ["dismiss"] }));

    expect(screen.getByText(/cannot be merged/)).toBeTruthy();
    expect(screen.queryByText("Which record should be kept?")).toBeNull();
  });

  // And the ordinary pair still asks its own question, or the case above is
  // satisfied by a card that stopped asking it to anybody.
  it("asks which record survives when the merge is on offer", () => {
    draw(pairRow());

    expect(screen.getByText("Which record should be kept?")).toBeTruthy();
    expect(screen.queryByText(/cannot be merged/)).toBeNull();
  });

  // The dismissal posts the same disposition from the unmergeable pair as from
  // an ordinary one — the server's not-a-duplicate arm takes write authority
  // and applies no project rule, which is the whole reason the verb is offered
  // here. A control that drew and posted nothing would pass the case above.
  it("settles an unmergeable pair as not a duplicate", async () => {
    const user = userEvent.setup();
    const fetched = stubOk();
    draw(pairRow({ actions: ["dismiss"] }));

    await user.click(screen.getByRole("button", { name: "Not duplicates" }));

    await vi.waitFor(() => expect(fetched).toHaveBeenCalled());
    const body = await bodyOf(fetched);
    expect(body.disposition).toBe("not_a_duplicate");
    expect(Object.hasOwn(body, "winner_id")).toBe(false);
  });

  // Two contacts merge only through the case review, where the reader ticks
  // both and sees the preview. Keep opens it on the record they chose.
  it("opens the review on the kept contact and posts nothing", async () => {
    const requests: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : new Request(input);
        requests.push(`${request.method} ${new URL(request.url).pathname}`);
        return new Response(JSON.stringify(contactCase({ anchor_id: BEN })), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }),
    );
    const user = userEvent.setup();
    draw(
      pairRow({
        kind: "contact",
        pair: {
          left: { id: ANA, label: "Mira Kovac" },
          right: { id: BEN, label: "Mira Kovač" },
          evidence: [],
        },
      }),
    );

    await user.click(screen.getByRole("button", { name: "Keep Mira Kovač" }));

    const review = await screen.findByRole("dialog", {
      name: "Possible duplicates",
    });
    expect(
      await within(review).findByRole("checkbox", { name: /Mira Kovač/ }),
    ).toBeChecked();
    expect(requests).toEqual([
      expect.stringMatching(new RegExp(`^GET .*/dedupe/contact-cases/${BEN}$`)),
    ]);
  });
});

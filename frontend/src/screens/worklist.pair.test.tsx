// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
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
    id: "01a05500-0000-7000-8000-0000000000ff",
    source: "dedupe_candidate",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: ["merge"],
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

function draw(item: WorklistItem) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
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

afterEach(() => {
  cleanup();
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

  // THE case this component exists for. Merging KEEPS one record and archives
  // the other, so the winner must be the one the reader pressed — picking for
  // them would be the product deciding which of a customer's records is real.
  it("merges into the record the reader chose", async () => {
    const fetched = stubOk();
    draw(pairRow());

    // BY NAME, not by position: this is the assertion that the button the
    // reader believes they pressed is the record that survives.
    await userEvent.click(
      screen.getByRole("button", { name: "Keep ACME Gmbh" }),
    );

    await vi.waitFor(() => expect(fetched).toHaveBeenCalled());
    const body = await bodyOf(fetched);
    expect(body.disposition).toBe("merge");
    expect(body.winner_id).toBe(RIGHT);
  });

  // The other answer settles the pair for everybody and needs no winner. The
  // server refuses `winner_id` here, so sending one would 422 a decision the
  // reader made correctly.
  it("sends no winner when the records are not the same", async () => {
    const fetched = stubOk();
    draw(pairRow());

    await userEvent.click(screen.getByRole("button", { name: "Not the same" }));

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
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: "conflict" }), {
            status: 409,
            headers: { "content-type": "application/problem+json" },
          }),
      ),
    );
    draw(pairRow());

    await userEvent.click(screen.getByRole("button", { name: "Not the same" }));

    expect(
      await screen.findByText(/could not be settled the way you asked/),
    ).toBeTruthy();
    // Never the claim that it is gone: a refused merge puts the pair back.
    expect(screen.queryByText(/gone from the list/)).toBeNull();
  });

  // A REFUSAL will refuse again however often it is pressed, so "try again" is
  // the one instruction that must not appear. This is the toast a rep met on
  // every pair spanning two owners.
  it("names the steward when the decision is refused, and never says try again", async () => {
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

    await userEvent.click(screen.getByRole("button", { name: "Not the same" }));

    expect(await screen.findByText(/cannot settle this pair/)).toBeTruthy();
    expect(screen.queryByText(/Try again/)).toBeNull();
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
    { what: "a pair somebody else settled", code: "conflict", status: 409 },
  ])(
    "keeps $what on screen until it is dismissed",
    async ({ code, status }) => {
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

      await userEvent.click(
        screen.getByRole("button", { name: "Not the same" }),
      );

      expect(await screen.findByRole("button", { name: "Close" })).toBeTruthy();
    },
  );

  // And the one that really is retryable still withdraws itself. Without this
  // the case above passes against a toast that never leaves, which would be a
  // different defect wearing the fix's clothes.
  it("lets an unexplained failure withdraw itself", async () => {
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

    await userEvent.click(screen.getByRole("button", { name: "Not the same" }));

    expect(await screen.findByText(/Try again/)).toBeTruthy();
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
  it("shows the pair without verbs when the server offered no merge", () => {
    draw(pairRow({ actions: [] }));

    expect(screen.getByText("Acme GmbH")).toBeTruthy();
    expect(screen.getByText("ACME Gmbh")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
    expect(
      screen.getByText(/Only somebody who can change both records/),
    ).toBeTruthy();
  });

  // The steward still gets the verbs. Without this the case above would pass
  // against a card that had simply stopped offering them to anybody.
  it("offers the verbs when the server offered merge", () => {
    draw(pairRow());

    expect(screen.getByRole("button", { name: "Keep Acme GmbH" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Not the same" })).toBeTruthy();
  });
});

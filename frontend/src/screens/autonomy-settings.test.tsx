/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { AutonomySettingsCard } from "./autonomy-settings";

// Settings → Account → what answers itself: the switches a rep sets on their own
// queue. No grant fixture appears below on purpose — the card reads and writes
// the reader's own rows, so there is no seat that could be refused one.

type Row = {
  kind: string;
  mode: "manual" | "veto" | "auto";
  approved_clean: number;
  approved_edited: number;
  rejected: number;
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function row(kind: string, mode: "manual" | "auto", clean = 0): Row {
  return {
    kind,
    mode,
    approved_clean: clean,
    approved_edited: 0,
    rejected: 0,
  };
}

// backendFor answers /autonomy with the given rows and applies a PATCH the way
// the server does — writing the mode and answering with the WHOLE set, which is
// the behaviour the card's cache update depends on.
function backendFor(rows: Row[]) {
  let state = rows;
  const patches: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({}));
      }
      if (req.url.includes("/autonomy")) {
        if (req.method === "PATCH") {
          const body = (await req.json()) as { kind: string; auto: boolean };
          patches.push(body);
          state = state.map((r) =>
            r.kind === body.kind
              ? { ...r, mode: body.auto ? "auto" : "manual" }
              : r,
          );
        }
        return jsonResponse({ data: state });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, patches: () => patches };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AutonomySettingsCard", () => {
  it("offers a kind the reader has never decided, switched off", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([row("close_date_correction", "manual")]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    // A setting that writes when you flip it is a switch, so its state is what
    // it announces rather than a DOM property.
    const toggle = await screen.findByTestId<HTMLButtonElement>(
      "autonomy-toggle-close_date_correction",
    );
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    // The offer stands on the description alone when there is no record, so the
    // card must say there is none rather than printing three zeroes.
    expect(
      screen.getByText(/have not decided one of these yet/i),
    ).not.toBeNull();
  });

  // A PERMANENT ZERO DESERVES A READING. The card is shown to every seat on
  // purpose — approvals are structural, so there is no role list to gate on —
  // which leaves a seat nothing is routed to looking at switches that will
  // never have anything to switch. "You have not decided one of these yet" is
  // true of one kind; it says nothing about why there is none of any kind.
  //
  // DECIDED, not "received": a seat may have work waiting on it right now, and
  // this card cannot see the queue.
  it("says why the record is empty when this seat has decided none of them", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        row("close_date_correction", "manual"),
        row("stage_correction", "manual"),
      ]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    expect(
      await screen.findByText(/have not decided any of these yet/i),
    ).not.toBeNull();
  });

  // And it goes away the moment one HAS. A note that stands beside a real track
  // record tells a working seat their queue is empty while they read its
  // history.
  it("says nothing of the sort once the seat has decided one", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        row("close_date_correction", "manual", 3),
        row("stage_correction", "manual"),
      ]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    expect(await screen.findByText(/3 approved as proposed/i)).not.toBeNull();
    expect(screen.queryByText(/have not decided any of these yet/i)).toBeNull();
  });

  // AND NOT WHEN THERE IS NOTHING TO HAVE DECIDED. "You have not decided any of
  // these yet" is a sentence about a set, and with no eligible kinds there is no
  // set — it reads as a queue the seat is behind on rather than an installation
  // that routes them nothing.
  it("says nothing of the sort when there is no kind to decide", async () => {
    vi.stubGlobal("fetch", backendFor([]).fetchMock);
    render(<AutonomySettingsCard />);

    // Awaited on the EMPTY STATE, which only the settled query can draw. The
    // card's own sub-copy renders before the fetch resolves, so waiting on that
    // would read the absence below off a card that had not loaded yet — an
    // assertion no implementation could fail.
    expect(await screen.findByText(/nothing here yet/i)).not.toBeNull();
    expect(screen.queryByText(/have not decided any of these yet/i)).toBeNull();
  });

  it("shows the track record under a kind the reader has decided", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([row("close_date_correction", "manual", 14)]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    expect(await screen.findByText(/14 approved as proposed/i)).not.toBeNull();
  });

  it("sends the kind it was switched on for, and reflects the answer", async () => {
    const backend = backendFor([
      row("close_date_correction", "manual"),
      row("lifecycle_change", "manual"),
    ]);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AutonomySettingsCard />);

    await user.click(
      await screen.findByTestId("autonomy-toggle-lifecycle_change"),
    );

    // The kind on the wire is the row's, not the first row's — the bug a shared
    // handler closing over the list would produce.
    await waitFor(() =>
      expect(backend.patches()).toEqual([
        { kind: "lifecycle_change", auto: true },
      ]),
    );
    await waitFor(() =>
      expect(
        screen
          .getByTestId("autonomy-toggle-lifecycle_change")
          .getAttribute("aria-checked"),
      ).toBe("true"),
    );
    // The other row is untouched by a write that never named it.
    expect(
      screen
        .getByTestId("autonomy-toggle-close_date_correction")
        .getAttribute("aria-checked"),
    ).toBe("false");
  });

  it("switches a kind back off", async () => {
    const backend = backendFor([row("org_name_promotion", "auto", 3)]);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AutonomySettingsCard />);

    const toggle = await screen.findByTestId<HTMLButtonElement>(
      "autonomy-toggle-org_name_promotion",
    );
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    await user.click(toggle);

    await waitFor(() =>
      expect(backend.patches()).toEqual([
        { kind: "org_name_promotion", auto: false },
      ]),
    );
    await waitFor(() =>
      expect(
        screen
          .getByTestId("autonomy-toggle-org_name_promotion")
          .getAttribute("aria-checked"),
      ).toBe("false"),
    );
  });

  it("says why when the server refuses the change", async () => {
    // The one arm a seeded screen cannot show. A rep who flips a switch and is
    // refused must be told; a card that swallowed the problem would leave them
    // believing a kind applies automatically when it does not.
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({}));
        }
        if (req.method === "PATCH") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Not Found",
              status: 404,
              code: "not_found",
              detail: "that kind does not apply automatically",
            },
            404,
          );
        }
        return jsonResponse({ data: [row("close_date_correction", "manual")] });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<AutonomySettingsCard />);

    await user.click(
      await screen.findByTestId("autonomy-toggle-close_date_correction"),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBeTruthy();
    // And the switch does not pretend the change landed.
    expect(
      screen
        .getByTestId("autonomy-toggle-close_date_correction")
        .getAttribute("aria-checked"),
    ).toBe("false");
  });

  it("shows a rung it cannot set as not-automatic, without claiming it is off", async () => {
    // `veto` is legal in the policy table and nothing writes it yet. The card
    // must not render it as checked — it does not apply on sight — and the
    // server keeps reporting the rep's real rung rather than this screen
    // rewriting it to manual.
    vi.stubGlobal(
      "fetch",
      backendFor([
        {
          kind: "close_date_correction",
          mode: "veto",
          approved_clean: 4,
          approved_edited: 0,
          rejected: 0,
        },
      ]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    const toggle = await screen.findByTestId(
      "autonomy-toggle-close_date_correction",
    );
    expect(toggle.getAttribute("aria-checked")).toBe("false");
  });

  it("renders a kind whose copy it does not carry", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([row("some_future_kind", "manual")]).fetchMock,
    );
    render(<AutonomySettingsCard />);

    // A choice the reader now has is worth showing unpolished. The switch is
    // what matters; the missing label falls back to its own key rather than
    // dropping the row.
    expect(
      await screen.findByTestId("autonomy-toggle-some_future_kind"),
    ).not.toBeNull();
  });
});

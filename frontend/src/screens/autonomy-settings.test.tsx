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
  mode: "manual" | "auto";
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
  let patches: unknown[] = [];
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

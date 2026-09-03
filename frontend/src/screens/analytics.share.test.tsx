/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ShareViewButton } from "./analytics.share";

// The share dialog's two obligations to a reader.
//
// One: the two kinds are told apart in WORDS, because a reader handed a frozen
// number without being told it is frozen reads a three-week-old figure as
// current. Two: the link is shown once and the dialog says what leaving costs,
// because nothing can read it back.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

function shareStub(token = "tok-abc") {
  return vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          id: "share-1",
          kind: "live",
          target: "forecast",
          expires_at: "2026-10-03T00:00:00Z",
          token,
          created_at: "2026-09-03T00:00:00Z",
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
  );
}

describe("sharing a forecast view", () => {
  it("distinguishes the live and frozen kinds in words", async () => {
    vi.stubGlobal("fetch", shareStub());
    // A frozen state EXISTS here, which is what makes both kinds offerable.
    render(<ShareViewButton target="forecast" snapshotId="snap-1" />);

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Both kinds named, and each one's promise stated. A label alone leaves a
    // reader guessing which of the two they were handed.
    expect(screen.getByLabelText(/Live view/)).toBeTruthy();
    expect(screen.getByText(/Recomputed each time it is opened/)).toBeTruthy();
    expect(
      screen.getByText(/as they stood when the state was taken/),
    ).toBeTruthy();
  });

  it("says the frozen kind is unavailable when nothing has been frozen", async () => {
    vi.stubGlobal("fetch", shareStub());
    render(<ShareViewButton target="forecast" />);

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Offered and then refused by the server is the shape to avoid: the reader
    // presses a choice, waits, and is told no.
    expect(
      screen.getByText("No state has been frozen for this period yet."),
    ).toBeTruthy();
  });

  it("shows the link once and says what leaving costs", async () => {
    vi.stubGlobal("fetch", shareStub("tok-xyz"));
    render(<ShareViewButton target="forecast" />);

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));

    const link = await screen.findByTestId("forecast-share-link");
    expect(link.textContent).toContain("tok-xyz");
    expect(screen.getByText(/only time the link is shown/)).toBeTruthy();
    expect(
      screen.getByText(/Leaving without copying discards the link/),
    ).toBeTruthy();
  });

  it("tells the reader to copy by hand when the clipboard refuses", async () => {
    vi.stubGlobal("fetch", shareStub());
    // No clipboard at all — an http origin, which is where this actually
    // happens. Silently doing nothing would leave the reader pressing Copy.
    vi.stubGlobal("navigator", { ...navigator, clipboard: undefined });
    render(<ShareViewButton target="forecast" />);

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Copy link" }),
    );

    expect(await screen.findByText(/could not be copied/)).toBeTruthy();
  });
});

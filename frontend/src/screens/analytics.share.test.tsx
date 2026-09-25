/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
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
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
        snapshotId="snap-1"
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Both kinds named, and each one's promise stated. A label alone leaves a
    // reader guessing which of the two they were handed.
    expect(screen.getByLabelText(/Live view/)).toBeTruthy();
    expect(screen.getByText(/Recalculated on each open/)).toBeTruthy();
    expect(
      screen.getByText(/as they stood when the snapshot was taken/),
    ).toBeTruthy();
  });

  it("says the frozen kind is unavailable when nothing has been frozen", async () => {
    vi.stubGlobal("fetch", shareStub());
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Offered and then refused by the server is the shape to avoid: the reader
    // presses a choice, waits, and is told no.
    expect(
      screen.getByText("No snapshot exists for this period yet."),
    ).toBeTruthy();
  });

  it("shows the link once and says what leaving costs", async () => {
    vi.stubGlobal("fetch", shareStub("tok-xyz"));
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));

    const link = await screen.findByTestId("forecast-share-link");
    expect(link.textContent).toContain("tok-xyz");
    expect(screen.getByText(/link is shown only once/)).toBeTruthy();
    expect(
      screen.getByText(/Leaving without copying discards the link/),
    ).toBeTruthy();
  });

  it("tells the reader to copy by hand when the clipboard refuses", async () => {
    vi.stubGlobal("fetch", shareStub());
    // No clipboard at all — an http origin, which is where this actually
    // happens. Silently doing nothing would leave the reader pressing Copy.
    stubClipboard("absent");
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Copy link" }),
    );

    expect(await screen.findByText(/clipboard access denied/i)).toBeTruthy();
    expect(screen.getByText(/copy it manually/i)).toBeTruthy();
  });

  it("closes the link it just issued, before its expiry", async () => {
    const issue = shareStub();
    const calls: Array<{ method: string; path: string }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        calls.push({
          method: request.method,
          path: new URL(request.url).pathname,
        });
        return request.method === "DELETE"
          ? new Response(null, { status: 204 })
          : issue();
      }),
    );
    const user = userEvent.setup();
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await user.click(await screen.findByRole("button", { name: "Close link" }));

    // The dialog stops offering a link that no longer opens.
    expect(await screen.findByText("Link closed")).toBeTruthy();
    expect(screen.queryByTestId("forecast-share-link")).toBeNull();
    expect(calls).toContainEqual({
      method: "DELETE",
      path: "/v1/forecast/shares/share-1",
    });
  });

  it("keeps the link on screen with the reason when closing is refused", async () => {
    const issue = shareStub("tok-kept");
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) =>
        request.method === "DELETE"
          ? new Response(
              JSON.stringify({
                title: "Forbidden",
                status: 403,
                detail: "Only the colleague who issued a share can close it.",
              }),
              {
                status: 403,
                headers: { "Content-Type": "application/problem+json" },
              },
            )
          : issue(),
      ),
    );
    const user = userEvent.setup();
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await user.click(await screen.findByRole("button", { name: "Close link" }));

    expect(
      await screen.findByText(/Only the colleague who issued a share/),
    ).toBeTruthy();
    expect(screen.getByTestId("forecast-share-link").textContent).toContain(
      "tok-kept",
    );
  });
});

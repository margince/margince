/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { DealsScreen } from "./deals";
import { deal, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

describe("the deals board under a search", () => {
  it("reads a search of only spaces as no search", async () => {
    window.location.hash = "#/deals?q=%20%20%20";
    const asked: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", name: "Fleet one", stage_id: "s1" })], {
        onDealsUrl: (url) => asked.push(url),
      }),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet one")).toBeTruthy();
    expect(asked.length).toBeGreaterThan(0);
    for (const url of asked) {
      expect(new URL(url, "https://t.l").searchParams.has("q")).toBe(false);
    }
  });

  it("says nothing matches and offers to clear when no column holds a card", async () => {
    window.location.hash = "#/deals?q=zzz";
    vi.stubGlobal("fetch", stubBackend([]));
    render(<DealsScreen />);

    expect(
      await screen.findByText(/No deals match these filters\./),
    ).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: englishCopy["aicalls.clear"] }),
    );
    await waitFor(() =>
      expect(screen.getByPlaceholderText(/Search/)).toHaveValue(""),
    );
  });

  it("draws no empty line over a board that holds cards", async () => {
    window.location.hash = "#/deals?q=fleet";
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", name: "Fleet one", stage_id: "s1" })]),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet one")).toBeTruthy();
    expect(screen.queryByText(/No deals match these filters\./)).toBeNull();
  });
});

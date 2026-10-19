/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CommandLineToolsCard } from "./commandline-tools";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function discovery(status: number) {
  return vi.fn(async () =>
    status === 200
      ? new Response(
          JSON.stringify({ resource: "https://crm.acme.test/mcp" }),
          { status, headers: { "Content-Type": "application/json" } },
        )
      : new Response("{}", { status }),
  );
}

function renderCard(status: number) {
  vi.stubGlobal("fetch", discovery(status));
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CommandLineToolsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("CommandLineToolsCard", () => {
  it("offers a command per tool, built from the URL the server advertises", async () => {
    renderCard(200);
    expect(
      await screen.findByText(
        "claude mcp add --transport http margince https://crm.acme.test/mcp",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "codex mcp add margince --url https://crm.acme.test/mcp",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Then run codex mcp login margince.")).toBeTruthy();
    // Antigravity refuses the `url`/`httpUrl` spellings; a wrong key is a
    // config that silently does nothing.
    expect(
      screen.getByText(/"serverUrl": "https:\/\/crm\.acme\.test\/mcp"/),
    ).toBeTruthy();
  });

  it("no longer offers Gemini CLI, which Antigravity replaced", async () => {
    renderCard(200);
    await screen.findByText(/claude mcp add/);
    expect(screen.queryByText(/gemini mcp add/)).toBeNull();
  });

  it("links each tool to its own docs", async () => {
    renderCard(200);
    const docs = await screen.findByRole("link", { name: /Codex docs/ });
    expect(docs.getAttribute("href")).toBe(
      "https://learn.chatgpt.com/docs/extend/mcp?surface=cli",
    );
    expect(
      screen
        .getByRole("link", { name: /Antigravity docs/ })
        .getAttribute("href"),
    ).toBe("https://antigravity.google/docs/mcp");
  });

  it("shows a failed discovery as an error with a retry, not an empty card", async () => {
    renderCard(500);
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
    expect(screen.queryByText(/claude mcp add/)).toBeNull();
  });

  it("says the connector is off instead of printing commands that cannot work", async () => {
    renderCard(404);
    await waitFor(() =>
      expect(
        screen.getByText("AI tools cannot connect to Margince yet."),
      ).toBeTruthy(),
    );
    expect(screen.queryByText(/claude mcp add/)).toBeNull();
  });
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CodingAgentsCard } from "./coding-agents";

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
        <CodingAgentsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("CodingAgentsCard", () => {
  it("shows one tool at a time, each command built from the URL the server advertises", async () => {
    renderCard(200);
    expect(
      await screen.findByText(
        "claude mcp add --transport http margince https://crm.acme.test/mcp",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Then run /mcp.")).toBeTruthy();
    expect(screen.queryByText(/codex mcp add/)).toBeNull();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Codex" }));
    expect(
      screen.getByText(
        "codex mcp add margince --url https://crm.acme.test/mcp",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Then run codex mcp login margince.")).toBeTruthy();
    expect(screen.queryByText(/claude mcp add/)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Antigravity" }));
    // Antigravity refuses the `url`/`httpUrl` spellings; a wrong key is a
    // config that silently does nothing.
    expect(
      screen.getByText(/"serverUrl": "https:\/\/crm\.acme\.test\/mcp"/),
    ).toBeTruthy();
    expect(
      screen.getByText(/choose “Authenticate” next to margince/),
    ).toBeTruthy();
  });

  it("no longer offers Gemini CLI, which Antigravity replaced", async () => {
    renderCard(200);
    await screen.findByText(/claude mcp add/);
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual(["Claude Code", "Codex", "Antigravity"]);
  });

  it("links the chosen tool to its own docs", async () => {
    renderCard(200);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Codex" }));
    expect(
      screen.getByRole("link", { name: /Codex docs/ }).getAttribute("href"),
    ).toBe("https://learn.chatgpt.com/docs/extend/mcp?surface=cli");
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

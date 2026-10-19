/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import * as download from "../../lib/download";
import { ConnectAiToolCard } from "./connect-ai-tool";

const MCP_URL = "https://crm.acme.test/mcp";

// user-event and the clipboard cases replace navigator.clipboard; each case
// starts from the browser's own.
const realClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (realClipboard) {
    Object.defineProperty(navigator, "clipboard", realClipboard);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
});

function discovery(status: number) {
  return vi.fn(async () =>
    status === 200
      ? new Response(JSON.stringify({ resource: MCP_URL }), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      : new Response("{}", { status }),
  );
}

function renderCard(status = 200) {
  vi.stubGlobal("fetch", discovery(status));
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectAiToolCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function choose(...names: string[]) {
  const user = userEvent.setup();
  for (const name of names) {
    await user.click(await screen.findByRole("button", { name }));
  }
  return user;
}

const pathBox = () => screen.getByTestId("aiconnect-path");
const steps = () =>
  within(pathBox())
    .queryAllByRole("listitem")
    .map((item) => item.textContent);

describe("ConnectAiToolCard", () => {
  it("opens on Claude for a company plan, with the admin first and no address to paste", async () => {
    renderCard();
    expect(
      (await screen.findByRole("button", { name: "Claude" })).getAttribute(
        "aria-pressed",
      ),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "My company" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen.getByText(
        "Your Claude owner adds Margince once for the whole team.",
      ),
    ).toBeTruthy();
    expect(steps()).toEqual([
      "In Claude, open “Customize”, then “Connectors”.",
      "Find Margince and choose “Connect”.",
      "Margince opens. Sign in and choose Authorize. Done.",
    ]);
    expect(
      screen.queryByRole("textbox", { name: "Margince address" }),
    ).toBeNull();
  });

  it("shows the address inside the step that asks the reader to paste it", async () => {
    renderCard();
    await choose("Only me");
    const address = screen.getByRole("textbox", { name: "Margince address" });
    expect(address.textContent).toBe(MCP_URL);
    const items = within(pathBox()).getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[1].contains(address)).toBe(true);
    expect(items[1].textContent).toContain("“Add custom connector”");
    expect(screen.getByText(/Free plan, Claude allows one/)).toBeTruthy();
  });

  it("says a path cannot connect instead of giving steps, and gives them back", async () => {
    renderCard();
    await choose("Microsoft Copilot", "Only me");
    expect(
      screen.getByText(/Copilot outside work cannot connect to other apps/),
    ).toBeTruthy();
    expect(steps()).toEqual([]);
    expect(screen.queryByRole("button", { name: /Download guide/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /Help from/ })).toBeNull();
    await choose("My company");
    expect(steps()).toHaveLength(3);
    for (const group of screen.getAllByRole("group")) {
      expect(
        within(group)
          .getAllByRole("button")
          .filter((button) => button.getAttribute("aria-pressed") === "true"),
      ).toHaveLength(1);
    }
  });

  it("downloads the admin guide built from the same path, with the real address", async () => {
    const save = vi
      .spyOn(download, "downloadBytes")
      .mockImplementation(() => undefined);
    renderCard();
    await choose("Download guide for your admin");
    expect(save).toHaveBeenCalledTimes(1);
    const [markdown, filename, type] = save.mock.calls[0];
    expect(filename).toBe("margince-setup-claude.md");
    expect(type).toMatch(/^text\/markdown/);
    expect(String(markdown)).toContain(MCP_URL);
    expect(String(markdown)).toContain("Find Margince and choose “Connect”.");
  });

  it("dates the guide on the day the steps were checked, in every zone", async () => {
    const real = Intl.DateTimeFormat().resolvedOptions();
    vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
      ...real,
      timeZone: "Pacific/Kiritimati",
    });
    const save = vi
      .spyOn(download, "downloadBytes")
      .mockImplementation(() => undefined);
    renderCard();
    await choose("Download guide for your admin");
    expect(String(save.mock.calls[0][0])).toMatch(/checked on 19\D10\D2026/);
  });

  it("copies the address, and says how to copy it by hand when the clipboard is missing", async () => {
    const writeText = vi.fn(async () => undefined);
    renderCard();
    const user = await choose("Gemini", "Only me");
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    await user.click(screen.getByRole("button", { name: "Copy address" }));
    expect(writeText).toHaveBeenCalledWith(MCP_URL);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Copied" })).toBeTruthy(),
    );
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: undefined,
    });
    // user-event installs its own clipboard on setup, so this case reuses one.
    await user.click(screen.getByRole("button", { name: "ChatGPT" }));
    await user.click(screen.getByRole("button", { name: "Copy address" }));
    expect(
      await screen.findByText("Select the address and copy it by hand."),
    ).toBeTruthy();
  });

  it("links to the vendor's own guide, and names the admin where the guide is theirs", async () => {
    renderCard();
    await choose("Gemini", "Only me");
    const help = screen.getByRole("link", { name: /Help from Google/ });
    expect(help.getAttribute("href")).toBe(
      "https://support.google.com/gemini/answer/17209137",
    );
    expect(
      screen.getByText("For now only in the US, in English, on the web."),
    ).toBeTruthy();
    await choose("Microsoft Copilot", "My company");
    expect(
      screen.getByRole("link", { name: /Help from Microsoft for your admin/ }),
    ).toBeTruthy();
  });

  it("says AI tools cannot connect yet instead of giving steps", async () => {
    renderCard(404);
    expect(
      await screen.findByText("AI tools cannot connect to Margince yet."),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Claude" })).toBeNull();
    expect(screen.queryByTestId("aiconnect-path")).toBeNull();
  });

  it("shows a failed discovery as an error, not an empty card", async () => {
    renderCard(500);
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
    expect(screen.queryByTestId("aiconnect-path")).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { locationDouble } from "../../testing/locationdouble";
import { ConnectFlow } from "./connect-flow";

// The connect dialog decides what the server is asked for. That covers which
// provider leads, whether its calendar is chained after it, and how far back
// the import reaches. These cases pin the body each answer sends, and the two
// ways out that never reach a vendor.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubConnect(answer: () => Response) {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requests.push(request);
      return answer();
    }),
  );
  return requests;
}

const consent = () =>
  jsonResponse({ authorize_url: "https://accounts.example/consent" });

type FlowProps = ComponentProps<typeof ConnectFlow>;

function render(props: Partial<FlowProps> = {}) {
  const onClose = vi.fn();
  const onImap = vi.fn();
  const client = new QueryClient();
  const tree = (open: boolean) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConnectFlow open={open} onClose={onClose} onImap={onImap} {...props} />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = rtlRender(tree(true));
  return { onClose, onImap, setOpen: (open: boolean) => rerender(tree(open)) };
}

async function bodyOf(request: Request) {
  return (await request.json()) as Record<string, unknown>;
}

function dialog() {
  return screen.getByRole("dialog");
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ConnectFlow from a vendor the page already picked", () => {
  it("opens on what to bring in, with Mail, Calendar and six months chosen", async () => {
    render({ startFrom: "google" });
    expect(
      screen.getByRole("heading", { name: "Connect Google" }),
    ).toBeTruthy();
    expect(screen.getByRole("checkbox", { name: /^Mail/ })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /^Calendar/ })).toBeChecked();
    expect(
      screen.getByRole("combobox", { name: "Import past email" }).textContent,
    ).toContain("6 months");
    const steps = within(
      screen.getByRole("list", { name: "Steps" }),
    ).getAllByRole("listitem");
    const current = steps.find(
      (step) => step.getAttribute("aria-current") === "step",
    );
    expect(current?.textContent).toContain("What to bring in");
    // The state reaches a screen reader as a word beside the title.
    expect(steps[0]?.textContent).toBe("Account Done");
  });

  it("asks for mail with the calendar chained and the import window", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    render({ startFrom: "google" });
    await user.click(screen.getByRole("button", { name: "Continue" }));
    // The press swapped its own button out; focus lands on the new title
    // rather than falling out of the dialog.
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { name: "Sign in with Google" }),
    );
    expect(screen.getByText(/Google asks twice/)).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(new URL(requests[0].url).pathname).toMatch(
      /\/connectors\/gmail\/connect$/,
    );
    expect(await bodyOf(requests[0])).toEqual({
      return_to: "settings",
      also_connect: "gcal",
      import_window: "6m",
    });
    await waitFor(() =>
      expect(globalThis.location.assign).toHaveBeenCalledWith(
        "https://accounts.example/consent",
      ),
    );
  });

  it("chains Microsoft's own calendar after its mail", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    render({ startFrom: "microsoft" });
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(
      screen.getByRole("button", { name: "Continue to Microsoft 365" }),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(new URL(requests[0].url).pathname).toMatch(
      /\/connectors\/graph\/connect$/,
    );
    expect(await bodyOf(requests[0])).toMatchObject({
      also_connect: "graphcal",
    });
  });

  it("leaves the calendar out once it is unticked", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    render({ startFrom: "google" });
    await user.click(screen.getByRole("checkbox", { name: /^Calendar/ }));
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.queryByText(/asks twice/)).toBeNull();
    expect(screen.queryByText("See and edit calendar events")).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(await bodyOf(requests[0])).toEqual({
      return_to: "settings",
      import_window: "6m",
    });
  });

  it("asks for no import when None is picked", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    render({ startFrom: "google" });
    await user.click(
      screen.getByRole("combobox", { name: "Import past email" }),
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "None",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(
      screen.getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(await bodyOf(requests[0])).toEqual({
      return_to: "settings",
      also_connect: "gcal",
    });
  });

  it("connects the calendar alone, with no import, when Mail is unticked", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    render({ startFrom: "google" });
    await user.click(screen.getByRole("checkbox", { name: /^Mail/ }));
    expect(
      screen.queryByRole("combobox", { name: "Import past email" }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(
      screen.getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(new URL(requests[0].url).pathname).toMatch(
      /\/connectors\/gcal\/connect$/,
    );
    expect(await bodyOf(requests[0])).toEqual({ return_to: "settings" });
  });

  it("refuses to go on with nothing ticked, and says why", async () => {
    const user = userEvent.setup();
    render({ startFrom: "google" });
    await user.click(screen.getByRole("checkbox", { name: /^Mail/ }));
    await user.click(screen.getByRole("checkbox", { name: /^Calendar/ }));
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    expect(screen.getByText("Tick Mail, Calendar or both.")).toBeTruthy();
  });

  it("offers only the calendar when the mailbox is already on the roster", () => {
    render({ startFrom: "google", connected: ["gmail"] });
    const mail = screen.getByRole("checkbox", { name: /^Mail/ });
    expect(mail).toBeDisabled();
    expect(mail).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /^Calendar/ })).toBeChecked();
  });

  it("says the vendor is not set up here when the connect answers 501", async () => {
    const user = userEvent.setup();
    stubConnect(() =>
      jsonResponse({ code: "not_implemented", detail: "not wired" }, 501),
    );
    render({ startFrom: "microsoft" });
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(
      screen.getByRole("button", { name: "Continue to Microsoft 365" }),
    );
    expect(
      await within(dialog()).findByText(
        "Microsoft 365 is not configured on this installation.",
      ),
    ).toBeTruthy();
    expect(globalThis.location.assign).not.toHaveBeenCalled();
  });

  it("reports a refused connect inside the dialog", async () => {
    const user = userEvent.setup();
    stubConnect(() => jsonResponse({ detail: "upstream down" }, 502));
    render({ startFrom: "google" });
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(
      screen.getByRole("button", { name: "Continue to Google" }),
    );
    const alert = await within(dialog()).findByRole("alert");
    expect(alert.textContent).toContain("upstream down");
    expect(globalThis.location.assign).not.toHaveBeenCalled();
  });
});

describe("ConnectFlow from the start", () => {
  it("asks for the account first and holds Continue until one is chosen", async () => {
    const user = userEvent.setup();
    render();
    expect(
      screen.getByRole("heading", { name: "Connect mail and calendar" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: /^Microsoft 365/ }));
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(
      screen.getByRole("heading", { name: "Connect Microsoft 365" }),
    ).toBeTruthy();
  });

  it("refuses an account this installation cannot connect, or one fully connected", () => {
    render({
      availability: [
        { provider: "gmail", reason: "ready" },
        { provider: "graph", reason: "app_missing" },
        { provider: "imap", reason: "ready" },
      ],
      connected: ["imap"],
    });
    expect(screen.getByRole("radio", { name: /^Google/ })).toBeEnabled();
    const microsoft = screen.getByRole("radio", { name: /^Microsoft 365/ });
    expect(microsoft).toBeDisabled();
    expect(microsoft.closest("label")?.textContent).toContain(
      "not configured on this installation",
    );
    const imap = screen.getByRole("radio", { name: /^Another mail host/ });
    expect(imap).toBeDisabled();
    expect(imap.closest("label")?.textContent).toContain("Already connected.");
  });

  it("hands another mail host to the IMAP form, closing itself first", async () => {
    const user = userEvent.setup();
    const requests = stubConnect(consent);
    const { onClose, onImap } = render();
    await user.click(screen.getByRole("radio", { name: /^Another mail host/ }));
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByRole("checkbox", { name: /^Calendar/ })).toBeDisabled();
    expect(screen.getByText("Not available over IMAP.")).toBeTruthy();
    expect(
      screen.queryByRole("combobox", { name: "Import past email" }),
    ).toBeNull();
    expect(screen.getByText(/cannot import old emails/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onImap).toHaveBeenCalledOnce();
    expect(onClose.mock.invocationCallOrder[0]).toBeLessThan(
      onImap.mock.invocationCallOrder[0],
    );
    expect(requests).toHaveLength(0);
  });

  it("goes back to the account with the pick kept", async () => {
    const user = userEvent.setup();
    render();
    await user.click(screen.getByRole("radio", { name: /^Google/ }));
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByRole("radio", { name: /^Google/ })).toBeChecked();
  });

  it("opens fresh each time rather than where the last visit stopped", async () => {
    const user = userEvent.setup();
    const { setOpen } = render();
    await user.click(screen.getByRole("radio", { name: /^Google/ }));
    await user.click(screen.getByRole("button", { name: "Continue" }));
    setOpen(false);
    setOpen(true);
    expect(
      await screen.findByRole("heading", { name: "Connect mail and calendar" }),
    ).toBeTruthy();
    expect(screen.getByRole("radio", { name: /^Google/ })).not.toBeChecked();
  });
});

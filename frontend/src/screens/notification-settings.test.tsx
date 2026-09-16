/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { type Locale, LocaleProvider } from "../i18n";
import { NotificationSettingsCard } from "./notification-settings";

// Settings → Me → Notifications: where each class of notice reaches this seat.
// No grant fixture appears below on purpose — the page reads and writes the
// reader's own rows, so there is no seat that could be refused one.

type Preference = { class: string; delivery: string; chosen: boolean };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The whole set, as the server always answers it: one row per class, every
// class, so a screen never has to remember the rows it is not looking at.
function everyClass(): Preference[] {
  return [
    { class: "approval_pending", delivery: "email", chosen: false },
    { class: "automation", delivery: "in_app", chosen: false },
    { class: "lead_sla", delivery: "in_app", chosen: false },
    { class: "capture", delivery: "in_app", chosen: false },
    { class: "system", delivery: "in_app", chosen: false },
    { class: "coach", delivery: "in_app", chosen: false },
  ];
}

// backendFor answers the preference list with the given rows and applies a PUT
// the way the server does — writing the one class named, marking it chosen, and
// answering with the WHOLE set, which is the behaviour the cache update
// depends on.
function backendFor(rows: Preference[]) {
  let state = rows;
  const puts: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({}));
      }
      if (req.url.includes("/me/notification-preferences")) {
        if (req.method === "PUT") {
          const body = (await req.json()) as {
            class: string;
            delivery: string;
          };
          puts.push(body);
          state = state.map((row) =>
            row.class === body.class
              ? { ...row, delivery: body.delivery, chosen: true }
              : row,
          );
        }
        return jsonResponse({ items: state });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, puts: () => puts };
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

describe("NotificationSettingsCard", () => {
  it("offers one choice per class the product sends", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    render(<NotificationSettingsCard />);

    expect(await screen.findAllByRole("combobox")).toHaveLength(6);
  });

  // A row the seat has never decided still shows what happens TODAY. The
  // default is the answer, so the control sits on it rather than on a blank or
  // an invented "Default" option — and approvals default to email while
  // everything else defaults to the app, so a screen showing one value for both
  // would be wrong about one of them.
  it("shows the standing default as the answer, not as a placeholder", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    render(<NotificationSettingsCard />);

    const approvals = await screen.findByRole("combobox", {
      name: /Approvals waiting on you/i,
    });
    expect(approvals.textContent).toContain("Email");
    expect(
      screen.getByRole("combobox", { name: /Automations that ran/i })
        .textContent,
    ).toContain("In the app");
  });

  // THE SERVER REFUSES `off` FOR COACHING with a 422, so the option is not
  // offered. Showing a choice that cannot be made is worse than not showing it:
  // the reader picks it, is refused, and learns nothing about why.
  it("does not offer to switch coaching off", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await user.click(
      await screen.findByRole("combobox", { name: /colleague's nudge/i }),
    );
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: "Off" })).toBeNull();
    for (const offered of ["In the app", "Email", "Daily digest"]) {
      expect(
        within(listbox).getByRole("option", { name: offered }),
      ).not.toBeNull();
    }
  });

  // Every other class MAY be switched off, so the coach rule above is a
  // statement about coaching and not about the control.
  it("offers Off on a class the server accepts it for", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await user.click(
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
    );
    expect(
      within(screen.getByRole("listbox")).getByRole("option", { name: "Off" }),
    ).not.toBeNull();
  });

  it("sends the class it was set on, and shows the server's answer", async () => {
    const backend = backendFor(everyClass());
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
      "Daily digest",
    );

    // The class on the wire is the row's, not the first row's — the bug a
    // shared handler closing over the list would produce.
    await waitFor(() =>
      expect(backend.puts()).toEqual([
        { class: "automation", delivery: "digest" },
      ]),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: /Automations that ran/i })
          .textContent,
      ).toContain("Daily digest"),
    );
    // The set came back whole and the untouched rows came back with it.
    expect(
      screen.getByRole("combobox", { name: /Approvals waiting on you/i })
        .textContent,
    ).toContain("Email");
  });

  it("says why when the server refuses the choice", async () => {
    // A reader who sets a delivery and is refused must be told. A page that
    // swallowed the problem would leave them believing a class is muted when
    // the server still sends it.
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({}));
        }
        if (req.method === "PUT") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Unprocessable Entity",
              status: 422,
              code: "validation_error",
              detail: "coaching cannot be switched off",
            },
            422,
          );
        }
        return jsonResponse({ items: everyClass() });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
      "Daily digest",
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("coaching cannot be switched off");
    // And the control does not pretend the choice landed.
    expect(
      screen.getByRole("combobox", { name: /Automations that ran/i })
        .textContent,
    ).toContain("In the app");
  });

  it("reads the rows in the reader's own language", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    render(<NotificationSettingsCard />, "de");

    expect(
      await screen.findByRole("combobox", { name: /Freigaben/i }),
    ).not.toBeNull();
  });
});

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
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { pickOption } from "../../design-system/select-testing";
import { type Locale, LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
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
      <LocaleProvider initial={locale}>
        <SavePageProvider>
          {ui}
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
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

  // The server's order is its own; the reader meets what waits on them first.
  it("reads approvals first and the installation's notices last", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    render(<NotificationSettingsCard />);

    const boxes = await screen.findAllByRole("combobox");
    const rows = boxes.map((box) =>
      box.closest("[data-testid]")?.getAttribute("data-testid"),
    );
    expect(rows).toEqual([
      "notification-delivery-approval_pending",
      "notification-delivery-coach",
      "notification-delivery-lead_sla",
      "notification-delivery-automation",
      "notification-delivery-capture",
      "notification-delivery-system",
    ]);
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
      await screen.findByRole("combobox", {
        name: en["notifications.class.coach.label"],
      }),
    );
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: "Off" })).toBeNull();
    for (const offered of ["In the app", "Daily digest"]) {
      expect(
        within(listbox).getByRole("option", { name: offered }),
      ).not.toBeNull();
    }
  });

  // THE SERVER REFUSES `email` FOR EVERY CLASS BUT APPROVALS, for the same
  // reason and with the same 422: it is the only class a producer stages a
  // message for, so the word would record a choice the product does not keep
  // and the reader would stop watching the screen.
  //
  // `Daily digest` is still offered, which is the half that keeps this a
  // statement about the ROUTE: every class can still reach a mailbox.
  it("offers immediate email on approvals alone, and the batch everywhere", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await user.click(
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
    );
    const automations = screen.getByRole("listbox");
    expect(
      within(automations).queryByRole("option", { name: "Email" }),
    ).toBeNull();
    expect(
      within(automations).getByRole("option", { name: "Daily digest" }),
    ).not.toBeNull();
    await user.keyboard("{Escape}");

    await user.click(
      screen.getByRole("combobox", { name: /Approvals waiting on you/i }),
    );
    expect(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Email",
      }),
    ).not.toBeNull();
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

  it("sends the class it was set on once Save is pressed, and shows the server's answer", async () => {
    const backend = backendFor(everyClass());
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
      "Daily digest",
    );
    expect(backend.puts()).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Save" }));

    // The class on the wire is the row's, not the first row's: the bug a
    // shared handler closing over the list would produce.
    await waitFor(() =>
      expect(backend.puts()).toEqual([
        { class: "automation", delivery: "digest" },
      ]),
    );
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save" })).toBeNull(),
    );
    expect(
      screen.getByRole("combobox", { name: /Automations that ran/i })
        .textContent,
    ).toContain("Daily digest");
    // The set came back whole and the untouched rows came back with it.
    expect(
      screen.getByRole("combobox", { name: /Approvals waiting on you/i })
        .textContent,
    ).toContain("Email");
  });

  it("writes two changed classes one after the other on one Save", async () => {
    const backend = backendFor(everyClass());
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
      "Off",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: /Approvals waiting on you/i }),
      "In the app",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(backend.puts()).toEqual([
        { class: "approval_pending", delivery: "in_app" },
        { class: "automation", delivery: "off" },
      ]),
    );
  });

  it("puts the stored delivery back on Discard and sends nothing", async () => {
    const backend = backendFor(everyClass());
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationSettingsCard />);
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Automations that ran/i }),
      "Daily digest",
    );
    await user.click(screen.getByRole("button", { name: "Discard" }));

    expect(
      screen.getByRole("combobox", { name: /Automations that ran/i })
        .textContent,
    ).toContain("In the app");
    expect(backend.puts()).toEqual([]);
  });

  it("says why when the server refuses the choice, and keeps it drafted", async () => {
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
    await user.click(screen.getByRole("button", { name: "Save" }));

    const alerts = await screen.findAllByRole("alert");
    // The bar carries the summary; the refused row also names itself, and its
    // control points at that notice.
    expect(alerts).toHaveLength(2);
    const select = screen.getByRole("combobox", {
      name: /Automations that ran/i,
    });
    const notice = alerts.find((alert) =>
      (select.getAttribute("aria-describedby") ?? "")
        .split(" ")
        .includes(alert.id),
    );
    expect(notice?.textContent).toContain(
      "“Automations that ran” was not saved",
    );
    expect(notice?.textContent).toContain("coaching cannot be switched off");
    expect(
      alerts.some((alert) =>
        alert.textContent?.includes("coaching cannot be switched off"),
      ),
    ).toBe(true);
    // The refused pick stays on the control and the bar stays open to retry.
    expect(
      screen.getByRole("combobox", { name: /Automations that ran/i })
        .textContent,
    ).toContain("Daily digest");
    expect(screen.getByRole("button", { name: "Save" })).not.toBeNull();
  });

  it("reads the rows in the reader's own language", async () => {
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    render(<NotificationSettingsCard />, "de");

    expect(
      await screen.findByRole("combobox", { name: /Freigaben/i }),
    ).not.toBeNull();
  });
});

describe("Browser notifications", () => {
  function browserAnswering(permission: NotificationPermission) {
    const requestPermission = vi.fn(async () => "granted" as const);
    vi.stubGlobal("Notification", { permission, requestPermission });
    vi.stubGlobal("fetch", backendFor(everyClass()).fetchMock);
    return requestPermission;
  }

  it("asks the browser only when the reader presses Allow", async () => {
    const requestPermission = browserAnswering("default");
    render(<NotificationSettingsCard />);
    const row = await screen.findByTestId("notification-browser");
    expect(requestPermission).not.toHaveBeenCalled();

    await userEvent.click(
      within(row).getByRole("button", {
        name: en["notifications.browser.allow"],
      }),
    );

    expect(requestPermission).toHaveBeenCalledTimes(1);
  });

  it("offers no Allow where the browser blocks notifications, and says how to change it", async () => {
    browserAnswering("denied");
    render(<NotificationSettingsCard />);
    const row = await screen.findByTestId("notification-browser");

    expect(within(row).queryByRole("button")).toBeNull();
    expect(row).toHaveTextContent(en["notifications.browser.denied"]);
  });
});

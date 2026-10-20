/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";
import { defaultOperations } from "./operationsettings.fixtures";

// Settings → System health: how often each background pass runs and how fast
// one mailbox sends. installation_settings:update changes them, and a value
// past a setting's bounds is refused in the box before the request.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SETTINGS_EDITOR: GrantSpec = {
  installation_settings: ["read", "update"],
};
const SETTINGS_READER: GrantSpec = { installation_settings: ["read"] };

// The request is flat and the record nests the operating values, so each
// wire field is filed where a read-back finds it.
function backendFor(allow: GrantSpec) {
  let operations = { ...defaultOperations };
  const patches: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (url.pathname.endsWith("/installation/settings")) {
        if (req.method === "PATCH") {
          const patch = await req.json();
          patches.push(patch);
          operations = { ...operations, ...(patch as object) };
        }
        return jsonResponse({ operations });
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  return { fetchMock, patches };
}

function render(
  node: ReactNode,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The schedules sit behind a summary; the send-pacing card has none to open.
async function openSchedules(user: UserEvent) {
  await user.click(
    await screen.findByText(translate("en", "operations.schedules.fold")),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("operation settings cards", () => {
  // Each row writes its own wire field and refuses on its own bounds: a row
  // wired to its neighbour's property would save into the wrong setting.
  it.each([
    [BackgroundSchedulesCard, "agent_runner_interval_seconds", "60", "9"],
    [BackgroundSchedulesCard, "webhook_retry_interval_seconds", "60", "3601"],
    [BackgroundSchedulesCard, "time_scan_interval_seconds", "600", "59"],
    [
      BackgroundSchedulesCard,
      "close_date_sweep_interval_seconds",
      "7200",
      "3599",
    ],
    [
      BackgroundSchedulesCard,
      "follow_up_reconcile_interval_seconds",
      "7200",
      "604801",
    ],
    [
      BackgroundSchedulesCard,
      "retention_sweep_interval_seconds",
      "43200",
      "3599",
    ],
    [BackgroundSchedulesCard, "geocode_backfill_interval_seconds", "0", "299"],
    [
      BackgroundSchedulesCard,
      "technical_backfill_interval_seconds",
      "0",
      "604801",
    ],
    [
      BackgroundSchedulesCard,
      "gmail_watch_scan_interval_seconds",
      "3600",
      "599",
    ],
    [
      BackgroundSchedulesCard,
      "graph_watch_scan_interval_seconds",
      "3600",
      "43201",
    ],
    [BackgroundSchedulesCard, "gmail_watch_renew_within_hours", "72", "145"],
    [BackgroundSchedulesCard, "graph_watch_renew_within_hours", "36", "23"],
    [SendPacingCard, "send_rate_limit", "10", "0"],
    [SendPacingCard, "send_rate_window_seconds", "120", "3601"],
    [SendPacingCard, "send_max_age_hours", "48", "169"],
  ] as const)(
    "%o saves %s and refuses past its bounds",
    async (Card, property, ok, bad) => {
      const backend = backendFor(SETTINGS_EDITOR);
      vi.stubGlobal("fetch", backend.fetchMock);
      const user = userEvent.setup();
      render(<Card />);
      if (Card === BackgroundSchedulesCard) {
        await openSchedules(user);
      }

      const box = await screen.findByTestId<HTMLInputElement>(
        `operation-${property}`,
      );
      expect(box.value).toBe(String(defaultOperations[property]));
      await user.clear(box);
      await user.type(box, `${bad}{Enter}`);
      expect(box).toHaveAttribute("aria-invalid", "true");
      expect(backend.patches).toEqual([]);

      await user.clear(box);
      await user.type(box, `${ok}{Enter}`);
      await waitFor(() =>
        expect(backend.patches).toEqual([{ [property]: Number(ok) }]),
      );
    },
  );

  // The range is not repeated on every row; a refusal names it in the row's
  // own unit, and the "0 is off" sweeps say so.
  it.each([
    [
      "close_date_sweep_interval_seconds",
      "59",
      "operations.refusal.seconds",
      { min: "3,600", max: "604,800" },
    ],
    [
      "geocode_backfill_interval_seconds",
      "299",
      "operations.refusal.secondsOrOff",
      { min: "300", max: "604,800" },
    ],
    [
      "gmail_watch_renew_within_hours",
      "145",
      "operations.refusal.hours",
      { min: "24", max: "144" },
    ],
  ] as const)(
    "names %s's range only once a typed value breaks it",
    async (property, bad, key, range) => {
      vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR).fetchMock);
      const user = userEvent.setup();
      render(<BackgroundSchedulesCard />);
      await openSchedules(user);

      const refusal = translate("en", key, range);
      const box = await screen.findByTestId<HTMLInputElement>(
        `operation-${property}`,
      );
      expect(box).not.toHaveAccessibleDescription(
        expect.stringContaining(range.max),
      );
      await user.clear(box);
      await user.type(box, `${bad}{Enter}`);
      expect(await screen.findByText(refusal)).toBeInTheDocument();
    },
  );

  it("folds the schedules behind one summary", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR).fetchMock);
    render(<BackgroundSchedulesCard />);

    const summary = await screen.findByText(
      translate("en", "operations.schedules.fold"),
    );
    expect(summary.closest("summary")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  // A bare 86400 hides a day. The reading names the box's unit and the span in
  // words, and a sweep's 0 reads as off.
  it("reads each interval in words under its box, as typed", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR).fetchMock);
    const user = userEvent.setup();
    render(<BackgroundSchedulesCard />);
    await openSchedules(user);

    const daily = await screen.findByTestId<HTMLInputElement>(
      "operation-close_date_sweep_interval_seconds",
    );
    expect(daily).toHaveAccessibleDescription(
      expect.stringContaining("86,400 seconds = 1 day"),
    );
    const renew = screen.getByTestId<HTMLInputElement>(
      "operation-gmail_watch_renew_within_hours",
    );
    expect(renew).toHaveAccessibleDescription(
      expect.stringContaining("= 2 days"),
    );
    await user.clear(daily);
    await user.type(daily, "5400");
    expect(daily).toHaveAccessibleDescription(
      expect.stringContaining("= 1 hour, 30 minutes"),
    );
    const geocode = screen.getByTestId<HTMLInputElement>(
      "operation-geocode_backfill_interval_seconds",
    );
    await user.clear(geocode);
    await user.type(geocode, "0");
    expect(geocode).toHaveAccessibleDescription(expect.stringContaining("Off"));
  });

  it("gives a count no reading", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR).fetchMock);
    render(<SendPacingCard />);

    const limit = await screen.findByTestId<HTMLInputElement>(
      "operation-send_rate_limit",
    );
    expect(limit).not.toHaveAccessibleDescription(expect.stringContaining("="));
  });

  it("is read-only to a seat without the update grant", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_READER).fetchMock);
    render(<SendPacingCard />);

    const limit = await screen.findByTestId<HTMLInputElement>(
      "operation-send_rate_limit",
    );
    expect(limit.disabled).toBe(true);
    expect(
      screen.getByText(translate("en", "operations.adminOnly")),
    ).toBeTruthy();
  });

  // The page opens on a job-health grant a custom role can hold alone; the
  // card then withholds itself rather than drawing a refusal.
  it("draws nothing and asks nothing of a seat that cannot read the settings", async () => {
    const backend = backendFor({});
    vi.stubGlobal("fetch", backend.fetchMock);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { container } = render(<BackgroundSchedulesCard />, client);

    await waitFor(() =>
      expect(client.getQueryState(["me"])?.status).toBe("success"),
    );
    await act(async () => {});
    expect(container.textContent).toBe("");
    expect(
      backend.fetchMock.mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes(
          "/installation/settings",
        ),
      ),
    ).toBe(false);
  });
});

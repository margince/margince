/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { viewerZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { HistoryBaselineRow } from "./connectors.historybaseline";

type CaptureConnection = components["schemas"]["CaptureConnection"];

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function connection(over: Partial<CaptureConnection> = {}): CaptureConnection {
  return {
    id: "c1",
    provider: "gmail",
    status: "connected",
    scopes: ["read"],
    can_backfill: true,
    ...over,
  };
}

function captureBaselineWrites() {
  const sent: unknown[] = [];
  installFetchStub({
    "PUT /connectors/gmail/history-baseline": (body: unknown) => {
      sent.push(body);
      return jsonResponse(connection());
    },
  });
  return sent;
}

it("saves the chosen date in the reader's own zone", async () => {
  const sent = captureBaselineWrites();
  const user = userEvent.setup();
  render(<HistoryBaselineRow conn={connection()} />);

  const save = screen.getByRole("button", {
    name: en["connectors.historyBaseline.save"],
  });
  expect(save.hasAttribute("disabled")).toBe(true);
  await user.type(
    screen.getByLabelText(en["connectors.historyBaseline.label"]),
    "01/03/2026",
  );
  await user.click(save);

  await waitFor(() =>
    expect(sent).toEqual([{ date: "2026-03-01", timezone: viewerZone() }]),
  );
});

it("sends nothing while the box holds text it cannot read", async () => {
  const sent = captureBaselineWrites();
  const user = userEvent.setup();
  render(
    <HistoryBaselineRow
      conn={connection({
        history_baseline: {
          date: "2026-03-01",
          timezone: "Europe/Berlin",
          set_at: "2026-03-02T09:00:00Z",
        },
      })}
    />,
  );
  const box = screen.getByLabelText(en["connectors.historyBaseline.label"]);

  await user.clear(box);
  await user.type(box, "99/99/9999");
  await user.click(
    screen.getByRole("button", { name: en["connectors.historyBaseline.save"] }),
  );

  expect(await screen.findByText("Enter a date as DD/MM/YYYY.")).toBeTruthy();
  expect(sent).toEqual([]);
});

it("states the stored date and zone, and clears it as null", async () => {
  const sent = captureBaselineWrites();
  const user = userEvent.setup();
  render(
    <HistoryBaselineRow
      conn={connection({
        history_baseline: {
          date: "2026-03-01",
          timezone: "Europe/Berlin",
          set_at: "2026-03-02T09:00:00Z",
        },
      })}
    />,
  );

  expect(
    screen.getByText(/Central European Standard Time · UTC\+1/),
  ).toBeTruthy();
  expect(screen.queryByText(/Europe\/Berlin/)).toBeNull();
  await user.click(
    screen.getByRole("button", {
      name: en["connectors.historyBaseline.clear"],
    }),
  );
  await waitFor(() => expect(sent).toEqual([{ date: null }]));
});

it("names the offset in force at midday of a daylight-change baseline", () => {
  render(
    <HistoryBaselineRow
      conn={connection({
        history_baseline: {
          date: "2026-03-29",
          timezone: "Europe/Berlin",
          set_at: "2026-03-30T09:00:00Z",
        },
      })}
    />,
  );

  expect(
    screen.getByText(/Central European Summer Time · UTC\+2/),
  ).toBeTruthy();
});

it("offers no clear verb when no date is set", () => {
  render(<HistoryBaselineRow conn={connection()} />);
  expect(
    screen.queryByRole("button", {
      name: en["connectors.historyBaseline.clear"],
    }),
  ).toBeNull();
});

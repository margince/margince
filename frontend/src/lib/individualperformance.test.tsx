/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import {
  mayCarryIndividualFigures,
  useFiguresFollowTheSwitch,
} from "./individualperformance";
import { INSTALLATION_SETTINGS_KEY } from "./uploadlimit";

const FIGURES_KEY = ["reporting-evaluation", "this-month"];

describe("what the switch reaches", () => {
  it("counts every surface that can carry a colleague's figures", () => {
    for (const key of [
      ["report", "pipeline-current"],
      ["reporting-evaluation", "x"],
      ["reporting-live"],
      ["forecast-assurance"],
      ["analytics-context"],
    ]) {
      expect(mayCarryIndividualFigures(key)).toBe(true);
    }
  });

  it("leaves reads that carry no figures alone", () => {
    for (const key of [
      INSTALLATION_SETTINGS_KEY,
      ["contact", "1"],
      ["deals"],
    ]) {
      expect(mayCarryIndividualFigures(key)).toBe(false);
    }
  });
});

// The policy that brings the installation read back when the reader returns is
// the query layer's, and queryclient.test.ts holds it.

const mounted = (client: QueryClient) => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useFiguresFollowTheSwitch(false), { wrapper });
};

it("drops figures already on screen when the switch goes off", async () => {
  const client = new QueryClient();
  client.setQueryData(INSTALLATION_SETTINGS_KEY, {
    individual_performance: true,
  });
  client.setQueryData(FIGURES_KEY, { total: 7 });
  mounted(client);

  client.setQueryData(INSTALLATION_SETTINGS_KEY, {
    individual_performance: false,
  });

  await waitFor(() =>
    expect(client.getQueryState(FIGURES_KEY)?.isInvalidated).toBe(true),
  );
});

// A session that opens with the switch already off has nothing to drop, and
// invalidating there would refetch every report on every sign-in.
it("drops nothing when a session opens with the switch already off", async () => {
  const client = new QueryClient();
  client.setQueryData(INSTALLATION_SETTINGS_KEY, {
    individual_performance: false,
  });
  client.setQueryData(FIGURES_KEY, { total: 7 });
  mounted(client);

  await waitFor(() => expect(client.getQueryData(FIGURES_KEY)).toBeDefined());
  expect(client.getQueryState(FIGURES_KEY)?.isInvalidated).toBe(false);
});

// Turning it back on leaves the cache alone: the figures a reader may see again
// are read when a screen asks, not dropped on the way in.
it("drops nothing when the switch goes back on", async () => {
  const client = new QueryClient();
  client.setQueryData(INSTALLATION_SETTINGS_KEY, {
    individual_performance: false,
  });
  client.setQueryData(FIGURES_KEY, { total: 7 });
  mounted(client);

  client.setQueryData(INSTALLATION_SETTINGS_KEY, {
    individual_performance: true,
  });

  await waitFor(() => expect(client.getQueryData(FIGURES_KEY)).toBeDefined());
  expect(client.getQueryState(FIGURES_KEY)?.isInvalidated).toBe(false);
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { dateTimePreferences } from "../format/preferences";
import { DateFormatsProvider } from "./dateformats";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("observes preferences without retrying the authenticated gate's failed request", async () => {
  const fetch = vi.fn().mockRejectedValue(new Error("settings unavailable"));
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <DateFormatsProvider>
        <p>Home</p>
      </DateFormatsProvider>
    </QueryClientProvider>,
  );
  await act(async () => {
    await Promise.resolve();
  });
  expect(fetch).not.toHaveBeenCalled();
  view.unmount();
  expect(dateTimePreferences()).toEqual({
    dateFormat: "locale",
    timeFormat: "locale",
  });
  client.clear();
});

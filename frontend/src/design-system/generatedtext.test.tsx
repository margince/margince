// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { GeneratedText } from "./generatedtext";

const id = "11111111-1111-4111-8111-111111111111";
const original = "Den Vertrag prüfen";
const translated = "Review the contract";
const display = {
  locale: "en",
  source_version: 7,
  generated_parts: ["subject"],
} as const;
const clients: QueryClient[] = [];
let visible = true;
const intersections: Array<(shown: boolean) => void> = [];
beforeEach(() => {
  visible = true;
  intersections.length = 0;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(
        private callback: (entries: Array<{ isIntersecting: boolean }>) => void,
      ) {}
      observe() {
        const notify = (shown: boolean) =>
          this.callback([{ isIntersecting: shown }]);
        intersections.push(notify);
        notify(visible);
      }
      disconnect() {}
    },
  );
});

afterEach(() => {
  cleanup();
  for (const client of clients) client.clear();
  clients.length = 0;
  vi.unstubAllGlobals();
});
function answer(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function client() {
  const cache = new QueryClient();
  cache.setQueryData(["me"], {
    ...meFixture(),
    reader_rendering_available: true,
  });
  clients.push(cache);
  return cache;
}
function draw(cache: QueryClient) {
  return render(
    <QueryClientProvider client={cache}>
      <LocaleProvider initial="en">
        <GeneratedText
          activityId={id}
          display={{ ...display, generated_parts: ["subject"] }}
        >
          {original}
        </GeneratedText>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
const rendering = {
  activity_id: id,
  source_version: 7,
  part: "subject",
  target_language: "en",
  subject: translated,
  unchanged: false,
  generated_at: "2026-11-09T00:00:00Z",
};

it("offers the original beside generated prose and keeps unmarked human text unchanged", async () => {
  const fetch = vi.fn(async () => answer(200, rendering));
  vi.stubGlobal("fetch", fetch);
  draw(client());
  expect(await screen.findByText(translated)).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: en["readerRendering.showOriginal"] }),
  );
  expect(screen.getByText(original)).toBeTruthy();
  expect(screen.queryByText(translated)).toBeNull();
  cleanup();
  render(<GeneratedText activityId={id}>Human instruction</GeneratedText>);
  expect(screen.getByText("Human instruction")).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("never shows retained translated data after a fresh access check refuses it", async () => {
  let deny = false;
  const fetch = vi.fn(async (request: Request) =>
    request.method === "POST"
      ? answer(200, rendering)
      : deny
        ? answer(403, { code: "permission_denied", status: 403 })
        : answer(200, {
            id,
            version: 7,
            system_display: { generated_parts: ["subject"] },
          }),
  );
  vi.stubGlobal("fetch", fetch);
  const cache = client();
  const first = draw(cache);
  expect(await screen.findByText(translated)).toBeTruthy();
  first.unmount();
  deny = true;
  draw(cache);
  await waitFor(() =>
    expect(fetch.mock.calls.map(([request]) => request.method)).toEqual([
      "POST",
      "GET",
    ]),
  );
  await screen.findByRole("button", { name: en["readerRendering.show"] });
  expect(screen.queryByText(translated)).toBeNull();
  expect(screen.getByText(original)).toBeTruthy();
});

it("does not request generated text until the row is visible", async () => {
  visible = false;
  const fetch = vi.fn(async () => answer(200, rendering));
  vi.stubGlobal("fetch", fetch);
  draw(client());
  expect(screen.getByText(original)).toBeTruthy();
  expect(fetch).not.toHaveBeenCalled();
  for (const intersect of intersections) intersect(true);
  expect(await screen.findByText(translated)).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
});

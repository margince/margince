/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { meFixture } from "../../testing/mefixture";
import { steppedClock } from "../../testing/steppedclock";
import { useSignOff } from "./composesignoff";

type Draft = components["schemas"]["EmailSignatureDraft"];
type Answer = components["schemas"]["EmailSignOff"];

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("hides superseded markup until the current draft answers, including a late old response", async () => {
  steppedClock();
  const pending: {
    draft: Draft;
    signal: AbortSignal;
    answer: (value: Answer) => void;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("Expected the generated client's request");
      const payload: components["schemas"]["EmailSignOffRequest"] = await input
        .clone()
        .json();
      return new Promise<Response>((resolve) => {
        pending.push({
          draft: payload.draft ?? {},
          signal: input.signal,
          answer: (value) =>
            resolve(
              new Response(JSON.stringify(value), {
                headers: { "Content-Type": "application/json" },
              }),
            ),
        });
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryDefaults(["me"], { staleTime: Infinity });
  client.setQueryData(["me"], meFixture({}));
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result, rerender } = renderHook(
    ({ html }: { html: string }) => useSignOff("", "", { html }),
    { wrapper, initialProps: { html: "<p>First</p>" } },
  );
  await waitFor(() => expect(pending).toHaveLength(1));
  await act(async () =>
    pending[0].answer({
      text: "First",
      html: "<p>First</p>",
      kind: "signature",
    }),
  );
  await waitFor(() => expect(result.current.signOff?.text).toBe("First"));
  rerender({ html: "<p>Second</p>" });
  expect(result.current.signOff).toBeUndefined();
  await act(() => vi.advanceTimersByTimeAsync(1000));
  await waitFor(() => expect(pending).toHaveLength(2));
  expect(pending[1].draft.html).toBe("<p>Second</p>");
  rerender({ html: "" });
  await act(() => vi.advanceTimersByTimeAsync(1000));
  await waitFor(() => expect(pending).toHaveLength(3));
  expect(pending[1].signal.aborted).toBe(true);
  expect(pending[2].draft.html).toBe("");
  await act(async () =>
    pending[2].answer({
      text: "Company",
      html: "<p>Company</p>",
      kind: "template",
    }),
  );
  await waitFor(() => expect(result.current.signOff?.text).toBe("Company"));
  await act(async () =>
    pending[1].answer({
      text: "Second",
      html: "<p>Second</p>",
      kind: "signature",
    }),
  );
  expect(result.current.signOff?.text).toBe("Company");
});

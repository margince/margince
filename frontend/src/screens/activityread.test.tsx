/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, test } from "vitest";
import { LocaleProvider } from "../i18n";
import { useActivity } from "./activityread";
import { installFetchStub, jsonResponse } from "./story-utils";

const MEETING = {
  id: "meeting-9",
  kind: "meeting",
  subject: "Akeneo — Vergleichsblatt",
  body: "Lena: Ich mache ein Vergleichsblatt mit zwei Produkten.",
  occurred_at: "2026-09-08T08:00:00Z",
  version: 1,
};

// One client for the whole test, so a re-render reads the cache the first
// render filled: that cached answer is what these hold back.
function readActivity(refused: () => boolean) {
  installFetchStub({
    "GET /activities/meeting-9": () =>
      refused()
        ? jsonResponse({ title: "Not found", status: 404 }, 404)
        : jsonResponse(MEETING),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(
    ({ enabled }: { enabled: boolean }) => useActivity("meeting-9", enabled),
    {
      initialProps: { enabled: true },
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">{children}</LocaleProvider>
        </QueryClientProvider>
      ),
    },
  );
}

test("a re-read refused after access is revoked answers nothing it had read", async () => {
  let refused = false;
  const { result } = readActivity(() => refused);
  await waitFor(() =>
    expect(result.current.data?.subject).toBe(MEETING.subject),
  );

  refused = true;
  await result.current.refetch();

  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(result.current.data).toBeUndefined();
});

test("a read switched off answers nothing it had read", async () => {
  const { result, rerender } = readActivity(() => false);
  await waitFor(() =>
    expect(result.current.data?.subject).toBe(MEETING.subject),
  );

  rerender({ enabled: false });

  expect(result.current.data).toBeUndefined();
});

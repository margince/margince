/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, test } from "vitest";
import { LocaleProvider } from "../i18n";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import {
  activityUnavailableQuery,
  useActivity,
  useReadableActivityList,
} from "./activityread";

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
function readActivity(refused: () => boolean, status = 404) {
  installFetchStub({
    "GET /activities/meeting-9": () =>
      refused()
        ? jsonResponse(
            {
              title: "Read failed",
              code: status === 404 ? "not_found" : "internal",
              status,
            },
            status,
          )
        : jsonResponse(MEETING),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const initialProps: { enabled: boolean; readable: readonly string[] } = {
    enabled: true,
    readable: [],
  };
  const hook = renderHook(
    ({
      enabled,
      readable,
    }: {
      enabled: boolean;
      readable: readonly string[];
    }) => {
      useReadableActivityList(readable);
      return useActivity("meeting-9", enabled);
    },
    {
      initialProps,
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">{children}</LocaleProvider>
        </QueryClientProvider>
      ),
    },
  );
  return { ...hook, client };
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

  rerender({ enabled: false, readable: [] });

  expect(result.current.data).toBeUndefined();
});

test("a transient failure keeps a readable source available for retry", async () => {
  let refused = false;
  const { result, client } = readActivity(() => refused, 500);
  await waitFor(() =>
    expect(result.current.data?.subject).toBe(MEETING.subject),
  );
  refused = true;
  await result.current.refetch();
  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(
    client.getQueryData(activityUnavailableQuery(MEETING.id).queryKey),
  ).toBe(false);
});

test("a fresh readable list restores a source after its access was refused", async () => {
  let refused = false;
  const { result, client, rerender } = readActivity(() => refused);
  await waitFor(() =>
    expect(result.current.data?.subject).toBe(MEETING.subject),
  );
  refused = true;
  await result.current.refetch();
  await waitFor(() =>
    expect(
      client.getQueryData(activityUnavailableQuery(MEETING.id).queryKey),
    ).toBe(true),
  );
  rerender({ enabled: false, readable: [MEETING.id] });
  await waitFor(() =>
    expect(
      client.getQueryData(activityUnavailableQuery(MEETING.id).queryKey),
    ).toBe(false),
  );
});

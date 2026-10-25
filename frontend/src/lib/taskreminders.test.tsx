/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { requestNotificationPermission } from "./notificationpermission";
import { removeStored, STORAGE_KEYS, writeStored } from "./storage";
import { reminderKey, useTaskReminderNotifications } from "./taskreminders";

const REMINDER = {
  id: "task-1",
  kind: "task",
  subject: "Call back about the renewal",
  remind_at: "2026-10-20T09:00:00Z",
  occurred_at: "2026-10-01T09:00:00Z",
  version: 1,
};

class FakeNotification {
  static permission: NotificationPermission = "granted";
  static requestPermission = vi.fn(async () => FakeNotification.permission);
  static shown: FakeNotification[] = [];
  onclick: (() => void) | null = null;
  constructor(
    readonly title: string,
    readonly options?: NotificationOptions,
  ) {
    FakeNotification.shown.push(this);
  }
  close() {}
}

let reads: URL[] = [];

function serve(rows: unknown[]) {
  reads = [];
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://app.local",
    );
    if (url.pathname.endsWith("/activities")) reads.push(url);
    return new Response(JSON.stringify({ data: rows, page: {} }), {
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(() => useTaskReminderNotifications(true), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    ),
  });
}

beforeEach(() => {
  FakeNotification.permission = "granted";
  FakeNotification.shown = [];
  vi.stubGlobal("Notification", FakeNotification);
  writeStored(STORAGE_KEYS.taskRemindersShown, "[]");
});

afterEach(() => {
  vi.unstubAllGlobals();
  removeStored(STORAGE_KEYS.taskRemindersShown);
});

test("a newly due reminder shows one notification naming the task", async () => {
  serve([REMINDER]);
  mount();

  await waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
  expect(FakeNotification.shown[0]?.title).toBe(REMINDER.subject);
  expect(FakeNotification.shown[0]?.options?.tag).toBe(reminderKey(REMINDER));
  expect(reads[0]?.searchParams.get("reminder_due")).toBe("true");
});

test("a reminder already shown in this browser is not shown again", async () => {
  writeStored(
    STORAGE_KEYS.taskRemindersShown,
    JSON.stringify([reminderKey(REMINDER)]),
  );
  serve([REMINDER]);
  mount();

  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => {});
  expect(FakeNotification.shown).toHaveLength(0);
});

test("a moved reminder on the same task is a new reminder", async () => {
  writeStored(
    STORAGE_KEYS.taskRemindersShown,
    JSON.stringify([reminderKey(REMINDER)]),
  );
  serve([{ ...REMINDER, remind_at: "2026-10-21T09:00:00Z" }]);
  mount();

  await waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
});

test("a browser that has shown none before shows none of the reminders already due", async () => {
  removeStored(STORAGE_KEYS.taskRemindersShown);
  serve([REMINDER]);
  mount();

  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => {});
  expect(FakeNotification.shown).toHaveLength(0);
});

test("with notifications refused, nothing is read and nothing is shown", async () => {
  FakeNotification.permission = "denied";
  serve([REMINDER]);
  mount();

  await act(async () => {});
  expect(reads).toHaveLength(0);
  expect(FakeNotification.shown).toHaveLength(0);
});

test("allowing notifications starts the read with no reload", async () => {
  FakeNotification.permission = "default";
  serve([REMINDER]);
  mount();
  await act(async () => {});
  expect(reads).toHaveLength(0);

  FakeNotification.permission = "granted";
  await act(async () => {
    await requestNotificationPermission();
  });

  await waitFor(() => expect(reads).toHaveLength(1));
});

test("clicking the notification opens the task on Home", async () => {
  serve([REMINDER]);
  mount();
  await waitFor(() => expect(FakeNotification.shown).toHaveLength(1));

  FakeNotification.shown[0]?.onclick?.();

  expect(globalThis.location.hash).toBe("#/home?task=task-1");
});

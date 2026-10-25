// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A due task reminder, shown as a browser notification while a Margince tab is
// open. The server decides which reminders are due (`reminder_due` on the
// activity list); this file only decides whether this browser has shown one.
//
// "Once" is kept per browser, keyed by the task and its reminder instant. The
// record is in storage every tab shares, written under a lock the tabs share. The
// Worklist is where a reminder stays; the notification is the nudge to look.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useNotificationPermission } from "./notificationpermission";
import { unwrap } from "./problem";
import { navigate } from "./router";
import { readStoredJson, STORAGE_KEYS, writeStored } from "./storage";
import { worklistKey } from "./workkeys";

type Activity = components["schemas"]["Activity"];

/** The Home address dial that opens one task. */
export const TASK_PARAM = "task";

const POLL_MS = 60_000;
const PAGE = 50;
const LOCK = "margince-task-reminders";

export function reminderKey(task: Pick<Activity, "id" | "remind_at">): string {
  return `task-reminder:${task.id}:${task.remind_at ?? ""}`;
}

function readShown(): Set<string> | null {
  const stored = readStoredJson(STORAGE_KEYS.taskRemindersShown, (value) =>
    Array.isArray(value)
      ? value.filter((key): key is string => typeof key === "string")
      : null,
  );
  return stored === null ? null : new Set(stored);
}

// Without Web Storage the set lives as long as the tab does.
let shownInTab: Set<string> | null = null;
let storageRefused = false;

/**
 * The reminders not yet shown in this browser, recorded as shown. A browser
 * that has never recorded any records the current ones and shows none. So
 * allowing notifications does not bring up a month of old reminders.
 */
export function takeUnshown(due: readonly Activity[]): Activity[] {
  const shown = readShown() ?? (storageRefused ? shownInTab : null);
  const unshown =
    shown === null ? [] : due.filter((task) => !shown.has(reminderKey(task)));
  // Only keys still due are kept, so the set stays one page long.
  const keep = due.map(reminderKey);
  shownInTab = new Set(keep);
  writeStored(STORAGE_KEYS.taskRemindersShown, JSON.stringify(keep));
  storageRefused = readShown() === null;
  return unshown;
}

function underTabLock<T>(work: () => T): Promise<T> {
  const locks = typeof navigator === "undefined" ? undefined : navigator.locks;
  if (!locks) return Promise.resolve(work());
  return locks.request(LOCK, () => work());
}

export function openTaskFromNotification(id: string): void {
  globalThis.focus();
  navigate({ screen: "home" }, new Map([[TASK_PARAM, id]]));
}

function show(task: Activity, t: (key: MessageKey) => string): void {
  const note = new Notification(
    task.subject ?? t("reminder.notification.untitled"),
    {
      tag: reminderKey(task),
      body: t("reminder.notification.body"),
    },
  );
  note.onclick = () => {
    note.close();
    openTaskFromNotification(task.id);
  };
}

/**
 * Polls the reader's due reminders while they are signed in and notifications
 * are allowed. Mounted once, in the signed-in shell.
 */
export function useTaskReminderNotifications(signedIn: boolean): void {
  const t = useT();
  const client = useQueryClient();
  const granted = useNotificationPermission() === "granted" && signedIn;
  const due = useQuery({
    queryKey: ["task-reminders"],
    enabled: granted,
    refetchInterval: POLL_MS,
    // A notification is most often for a background tab.
    refetchIntervalInBackground: true,
    queryFn: async () =>
      unwrap(
        await api.GET("/activities", {
          params: { query: { reminder_due: true, kind: "task", limit: PAGE } },
        }),
        t,
      ).data,
  });
  useEffect(() => {
    if (!granted || !due.data) return;
    const page = due.data;
    void underTabLock(() => takeUnshown(page)).then((unshown) => {
      for (const task of unshown) show(task, t);
      if (unshown.length > 0)
        void client.invalidateQueries({ queryKey: worklistKey });
    });
  }, [granted, due.data, client, t]);
}

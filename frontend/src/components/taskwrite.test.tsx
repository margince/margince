/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { parseInstallHint } from "../lib/installhint";
import { readStoredJson, removeStored, STORAGE_KEYS } from "../lib/storage";
import { useTaskDone } from "./taskwrite";

afterEach(() => {
  cleanup();
  removeStored(STORAGE_KEYS.installHint);
  history.replaceState(null, "", "#/home");
  vi.unstubAllGlobals();
});

function answerPatch(status: number): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      status === 200
        ? Response.json({ id: "task-1", version: 2 })
        : Response.json(
            { code: "version_skew", detail: "The task changed." },
            { status },
          ),
    ),
  );
}

async function completeOn(hash: string): Promise<boolean> {
  history.replaceState(null, "", hash);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { result } = renderHook(() => useTaskDone("task-1", 1), {
    wrapper: ({ children }: Readonly<{ children: ReactNode }>) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    ),
  });
  let completed = false;
  await act(async () => {
    completed = await result.current.complete();
  });
  return completed;
}

const taskDoneOnHome = () =>
  readStoredJson(STORAGE_KEYS.installHint, parseInstallHint)?.taskDone ?? false;

it("notes a task completed on Home for the install hint", async () => {
  answerPatch(200);
  expect(await completeOn("#/home")).toBe(true);
  expect(taskDoneOnHome()).toBe(true);
});

it("notes nothing for a task completed off Home", async () => {
  answerPatch(200);
  expect(await completeOn("#/worklist")).toBe(true);
  expect(taskDoneOnHome()).toBe(false);
});

it("notes nothing when the write is refused", async () => {
  answerPatch(409);
  expect(await completeOn("#/home")).toBe(false);
  expect(taskDoneOnHome()).toBe(false);
});

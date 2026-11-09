// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// V8 counts coverage per isolate. A covered run counts every block the
// TypeScript checker runs, so a typed sweep of the tree takes three to four
// times as long. A worker thread is an isolate the coverage run never starts,
// so a sweep that loads no product code runs there at full speed.

import { Worker } from "node:worker_threads";

const ENTRY = new URL("./in-worker.entry.ts", import.meta.url);

// The function may be async; its result has to survive structured cloning.
type Call = (...args: never[]) => unknown;

/** Calls `name` from the module at `module` in a worker thread. */
export function inWorker<M extends Record<K, Call>, K extends string & keyof M>(
  module: URL,
  name: K,
  ...args: Parameters<M[K]>
): Promise<Awaited<ReturnType<M[K]>>> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(ENTRY, {
      workerData: { module: module.href, name, args },
    });
    worker.once("message", resolve);
    worker.once("error", reject);
    worker.once("exit", (code) => {
      reject(
        new Error(
          `inWorker: ${name} exited with code ${code} before answering`,
        ),
      );
    });
  });
}

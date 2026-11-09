// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The thread inWorker starts: it imports one module, calls one export and
// posts the result back. A throw ends the thread and reaches inWorker's caller.

import { parentPort, workerData } from "node:worker_threads";

const { module, name, args } = workerData;
const exported = (await import(module))[name];
if (typeof exported !== "function") {
  throw new Error(`inWorker: ${module} exports no function ${name}`);
}
parentPort?.postMessage(await exported(...args));

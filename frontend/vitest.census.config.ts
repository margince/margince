// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The census lane: census/census.test.ts walks and parses the tree once and
// runs every rule over it; census/rules/*.test.ts hold each rule's planted
// cases. No DOM and no setup file, because no case here renders anything.

import { defineConfig } from "vitest/config";
import { shuffledTests } from "./scripts/vitest-shards.ts";
import base from "./vite.config.ts";
import { TEST_TIMEOUT_MS } from "./vitest.budget.ts";

export default defineConfig({
  ...base,
  test: {
    name: "census",
    environment: "node",
    include: ["census/**/*.test.ts"],
    pool: "threads",
    testTimeout: TEST_TIMEOUT_MS,
    hookTimeout: TEST_TIMEOUT_MS,
    sequence: shuffledTests(process.env.TEST_SHUFFLE_SEED),
  },
});

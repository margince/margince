// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { type Locale, translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { periodWords } from "./companies.hierarchy.period";

const reader =
  (locale: Locale) => (key: MessageKey, params?: Record<string, string>) =>
    translate(locale, key, params);

it("reads a quarter as a sentence in the reader's language, and a year as it is", () => {
  expect(periodWords("2026-Q4", reader("vi"))).toBe("quý 4/2026");
  expect(periodWords("FY2025/26-Q1", reader("en"))).toBe("Q1 FY2025/26");
  expect(periodWords("2026", reader("vi"))).toBe("2026");
});

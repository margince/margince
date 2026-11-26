// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { recordNamesAnswer } from "./recordnames.testkit";

export function companyNamesAnswer(
  url: string,
  names: Readonly<Record<string, string>>,
): Response | null {
  const asked = new URL(url, "https://test.local");
  return asked.searchParams.get("kind") === "company"
    ? recordNamesAnswer(asked, { company: names })
    : null;
}

/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { readingsDay } from "../../testing/brief.fixtures";
import {
  BRIEF_READING_SLOTS,
  BriefReadingsPending,
  BriefReadingsStrip,
} from "./brief.readings";

afterEach(cleanup);

// The rail holds the strip's room while the day is read. A slot more or fewer
// than the strip draws is a placeholder that moves the meetings when it lands.
it("holds one slot per reading the strip draws", () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const strip = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefReadingsStrip day={readingsDay({})} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const drawn = strip.container.querySelectorAll(".stat-card").length;
  strip.unmount();
  const pending = render(<BriefReadingsPending />);
  expect(drawn).toBe(BRIEF_READING_SLOTS.length);
  expect(pending.container.querySelectorAll(".skeleton")).toHaveLength(drawn);
});

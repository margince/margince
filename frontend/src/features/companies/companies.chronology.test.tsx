// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ACTIVITY_KINDS } from "../../design-system/recordtimeline";
import { isConversationKind } from "../../design-system/timelineentry";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { company, company360 } from "../../testing/company.fixtures";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { stubWithSession } from "../../testing/story-utils";
import { useChronologySlots } from "./companies.chronology";

type Company360 = components["schemas"]["Company360"];

// The account read by someone whose grant reaches none of its activities.
const withheld: Company360 = {
  ...company360,
  activities: undefined,
  sections_omitted: ["activities"],
};

function History({ view }: Readonly<{ view: Company360 }>) {
  const { slots } = useChronologySlots({
    company,
    view,
    loading: false,
    failed: false,
    active: true,
  });
  return (
    <>
      {slots.timelineHeader}
      {slots.timelineNotice}
    </>
  );
}

let reads: URL[] = [];

beforeEach(() => {
  pretendReaderZone(UTC_ZONE);
  stubWithSession({}, {});
  const routed = globalThis.fetch;
  reads = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    if (url.pathname.endsWith("/activities")) reads.push(url);
    return routed(input, init);
  });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function draw(view: Company360) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <History view={view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a company history the reader's role withholds", () => {
  it("says so on All rather than reporting a failed load", async () => {
    draw(withheld);

    expect(await screen.findByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText(/did not load/)).toBeNull();
  });

  it("says so on the Threads cut too, and asks the server for nothing", async () => {
    const user = userEvent.setup();
    draw(withheld);

    await user.click(await screen.findByRole("button", { name: "Threads" }));

    expect(await screen.findByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText("No threads yet.")).toBeNull();
    expect(reads).toEqual([]);
  });
});

describe("the Threads cut of a company history", () => {
  it("asks the server for the conversation kinds", async () => {
    const user = userEvent.setup();
    draw(company360);

    await user.click(await screen.findByRole("button", { name: "Threads" }));

    await waitFor(() =>
      expect(reads.at(-1)?.searchParams.getAll("kind")).toEqual(
        ACTIVITY_KINDS.filter((kind) => isConversationKind(kind)),
      ),
    );
  });
});

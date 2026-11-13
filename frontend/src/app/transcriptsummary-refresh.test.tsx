/** @vitest-environment happy-dom */
import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TranscriptSummary } from "../design-system/transcriptsummary";
import { LocaleProvider } from "../i18n";
import { transcriptReadLatestKey } from "../lib/activitykeys";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { createQueryClient } from "./queryclient";

const clients: QueryClient[] = [];
const fetchBefore = globalThis.fetch;

afterEach(() => {
  cleanup();
  for (const client of clients) client.clear();
  clients.length = 0;
  globalThis.fetch = fetchBefore;
});

const reading = {
  read_id: "rd-1",
  activity_id: "a-1",
  status: "done",
  line_count: 4,
  proposal_ids: [],
  proposals: [],
  created_at: "2026-08-01T09:00:00Z",
  summary_refused: false,
  summary: {
    read_id: "rd-1",
    generated_at: "2026-08-01T09:01:00Z",
    key_points: [{ text: "Volumes rose fifteen percent.", source_lines: [3] }],
    next_steps: [],
  },
};

it("drops an erased transcript's summary from an open page after the next write", async () => {
  let erased = false;
  installFetchStub({
    "GET /activities/a-1/transcript-proposals/latest": () =>
      erased
        ? jsonResponse({ title: "Not Found" }, 404)
        : jsonResponse(reading),
  });
  const client = createQueryClient();
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <TranscriptSummary activityId="a-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("Volumes rose fifteen percent.")).toBeTruthy();

  // The erasure deleted the reading; any write on this page then re-reads it.
  erased = true;
  await act(() =>
    client
      .getMutationCache()
      .build(client, { mutationFn: async () => "erased" })
      .execute(undefined),
  );

  await waitFor(() =>
    expect(screen.queryByText("Volumes rose fifteen percent.")).toBeNull(),
  );
  expect(client.getQueryData(transcriptReadLatestKey("a-1"))).toBeNull();
});

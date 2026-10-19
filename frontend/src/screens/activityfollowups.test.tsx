/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { ActivityFollowUps } from "./activityfollowups";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  vi.useRealTimers();
});
const me = () =>
  jsonResponse(meFixture({ allow: { activity: ["read", "create"] } }));

it("continues the source's task list and opens the chosen task", async () => {
  let page = 0;
  installFetchStub({
    "GET /me": me,
    "GET /activities": () =>
      jsonResponse(
        ++page === 1
          ? {
              data: [
                {
                  id: "first",
                  kind: "task",
                  subject: "Send quote",
                  is_done: false,
                },
              ],
              page: { has_more: true, next_cursor: "next-page" },
            }
          : {
              data: [
                {
                  id: "second",
                  kind: "task",
                  subject: "Confirm receipt",
                  is_done: true,
                },
              ],
              page: { has_more: false, next_cursor: null },
            },
      ),
  });
  const fetchSpy = vi.spyOn(globalThis, "fetch");
  const onOpen = vi.fn();
  const user = userEvent.setup();
  renderWithQueryClient(
    <ActivityFollowUps
      sourceId="meeting-one"
      archived={false}
      onOpen={onOpen}
    />,
  );
  await screen.findByRole("button", { name: "Send quote" });
  await user.click(screen.getByRole("button", { name: "Load more" }));
  await user.click(
    await screen.findByRole("button", { name: "Confirm receipt" }),
  );
  expect(onOpen).toHaveBeenCalledWith("second");
  expect(screen.getByText("Completed")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  const urls = fetchSpy.mock.calls.map(
    ([input]) =>
      new URL(
        input instanceof Request ? input.url : String(input),
        "https://example.test",
      ),
  );
  expect(
    urls.some(
      (url) =>
        url.searchParams.get("source_activity_id") === "meeting-one" &&
        url.searchParams.get("cursor") === "next-page",
    ),
  ).toBe(true);
});

it("adds a follow-up from an unfiled source through the existing task form", async () => {
  vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": me,
    "GET /activities": () =>
      jsonResponse({ data: [], page: { has_more: false } }),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "new-task" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ActivityFollowUps
      sourceId="note-one"
      archived={false}
      onOpen={() => {}}
    />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Add a follow-up task" }),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Send scope" },
  });
  await user.click(screen.getByRole("button", { name: "Add task" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    kind: "task",
    source_activity_id: "note-one",
    subject: "Send scope",
    links: [],
  });
});

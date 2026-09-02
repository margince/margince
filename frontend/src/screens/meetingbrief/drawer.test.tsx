/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, StoryProviders } from "../story-utils";
import { PersonMeetingBrief } from "./drawer";
import { briefReady } from "./fixtures";

// What the DRAWER owns, as against the view: which URL goes out, and what the
// reader's project choice does to it.
//
// Mounting the drawer proves only that a callback fires with an id — the
// component could still ask for a different meeting, which is exactly the bug
// this surface once had. So these assert the request that actually leaves.

afterEach(cleanup);

// The shared installFetchStub hands a handler the request BODY, so it cannot
// answer "which URL went out" — which is the whole question here. The local
// recorder is the pattern the other screen tests use for the same reason.
function seen(routes: Record<string, () => Response>): string[] {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/v1/, "");
      urls.push(path + url.search);
      const handler = routes[`${request.method} ${path}`];
      return handler ? handler() : jsonResponse({ data: [], page: {} });
    }),
  );
  return urls;
}

describe("the meeting brief drawer", () => {
  it("asks for the meeting it was opened on", async () => {
    const urls = seen({
      "GET /activities/a-1/meeting-brief": () => jsonResponse(briefReady),
    });
    render(
      <StoryProviders>
        <PersonMeetingBrief activityId="a-1" open onClose={() => {}} />
      </StoryProviders>,
    );
    await waitFor(() => expect(urls).toHaveLength(1));
    expect(urls[0]).toBe("/activities/a-1/meeting-brief");
  });

  it("asks for nothing while it is closed", () => {
    const urls = seen({
      "GET /activities/a-1/meeting-brief": () => jsonResponse(briefReady),
    });
    render(
      <StoryProviders>
        <PersonMeetingBrief activityId="a-1" open={false} onClose={() => {}} />
      </StoryProviders>,
    );
    expect(urls).toHaveLength(0);
  });

  it("forgets a project chosen for one meeting when another opens", async () => {
    const urls = seen({
      "GET /activities/a-1/meeting-brief": () => jsonResponse(briefReady),
      "GET /activities/a-2/meeting-brief": () => jsonResponse(briefReady),
    });
    const projects = [
      {
        project_id: "p-erp",
        name: "ERP rollout",
        key: "ERP-27",
        phase: "delivering" as const,
      },
    ];
    const view = render(
      <StoryProviders>
        <PersonMeetingBrief
          activityId="a-1"
          open
          onClose={() => {}}
          projects={projects}
        />
      </StoryProviders>,
    );
    await waitFor(() => expect(urls).toHaveLength(1));
    const user = userEvent.setup();
    await user.click(await screen.findByRole("combobox", { name: "Project" }));
    await user.click(screen.getByRole("option", { name: /ERP-27/ }));
    await waitFor(() => expect(urls).toHaveLength(2));
    expect(urls[1]).toBe("/activities/a-1/meeting-brief?project_id=p-erp");

    // A scope chosen for one room must not narrow the brief for the next: the
    // same drawer is reused for the next meeting on the page.
    view.rerender(
      <StoryProviders>
        <PersonMeetingBrief
          activityId="a-2"
          open
          onClose={() => {}}
          projects={projects}
        />
      </StoryProviders>,
    );
    await waitFor(() => expect(urls).toHaveLength(3));
    expect(urls[2]).toBe("/activities/a-2/meeting-brief");
  });

  it("retries the read the reader asked to retry", async () => {
    let attempts = 0;
    const urls = seen({
      "GET /activities/a-1/meeting-brief": () => {
        attempts += 1;
        return attempts === 1
          ? jsonResponse({ status: 503, code: "unavailable" }, 503)
          : jsonResponse(briefReady);
      },
    });
    render(
      <StoryProviders>
        <PersonMeetingBrief activityId="a-1" open onClose={() => {}} />
      </StoryProviders>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Try again" }));
    await waitFor(() => expect(urls).toHaveLength(2));
    expect(await screen.findByText("Goal for this meeting")).toBeTruthy();
  });
});

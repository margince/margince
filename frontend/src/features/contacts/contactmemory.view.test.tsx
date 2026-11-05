/** @vitest-environment happy-dom */

// View on the What was said card opens a call, note or meeting whole in the
// shell's activity drawer. Closing the drawer returns to the card.

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { SourceHost } from "../../testing/sourcehost";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactMemory } from "./contactmemory";

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

afterEach(cleanup);

const ENTRY_ID = "01a05500-0000-7000-8000-00000000ee12";
const WORDS = "Wants the fleet numbers before we talk pricing.";

function entry(over: Partial<Activity> = {}): Activity {
  return {
    occurred_precision: "instant",
    offers_email: false,
    id: ENTRY_ID,
    kind: "note",
    occurred_at: "2026-08-28T11:00:00Z",
    subject: "Call prep",
    body: WORDS,
    content_state: "available",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-28T11:00:00Z",
    updated_at: "2026-08-28T11:00:00Z",
    version: 1,
    is_done: true,
    ...over,
  };
}

function viewWith(
  activities: Activity[],
  over: Partial<Contact360> = {},
): Contact360 {
  return {
    as_of: "2026-08-30T09:00:00Z",
    contact: {
      id: "p-1",
      full_name: "Dana Buyer",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-01T08:00:00Z",
      emails: [],
    },
    sections_omitted: [],
    activities: {
      data: activities,
      page: { has_more: false, next_cursor: null },
    },
    ...over,
  };
}

// The drawer reads the activity itself, so the stub's answer is what it shows.
function serve(answer: () => Response) {
  let reads = 0;
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    [`GET /activities/${ENTRY_ID}`]: () => {
      reads += 1;
      return answer();
    },
  });
  return () => reads;
}

function renderCard(view: Contact360) {
  return render(
    <StoryProviders>
      <SourceHost>
        <ContactMemory
          view={view}
          onOpenEmail={() => {}}
          onFullHistory={() => {}}
        />
      </SourceHost>
    </StoryProviders>,
  );
}

describe("View on the What was said card", () => {
  it.each([
    ["note", "Call prep"],
    ["call", "Pricing call"],
    ["meeting", "Fleet review"],
  ] as const)(
    "opens a %s whole and returns to the card",
    async (kind, subject) => {
      const user = userEvent.setup();
      serve(() => jsonResponse(entry({ kind, subject })));
      renderCard(viewWith([entry({ kind, subject })]));

      await user.click(screen.getByRole("button", { name: `View ${subject}` }));
      expect(await screen.findByText(WORDS)).toBeTruthy();
      expect(screen.getByRole("dialog")).toBeTruthy();

      await user.keyboard("{Escape}");
      await waitFor(() => {
        expect(screen.queryByText(WORDS)).toBeNull();
      });
      expect(
        screen.getByRole("button", { name: `View ${subject}` }),
      ).toBeTruthy();
    },
  );

  it("offers no View on an entry the reader may not read", () => {
    serve(() => jsonResponse(entry()));
    renderCard(
      viewWith([
        entry({ content_state: "withheld", subject: null, body: null }),
      ]),
    );

    expect(screen.getByText("Not shared with you")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^View/ })).toBeNull();
  });

  it("reads the entry again on every open, so lost access shows no old words", async () => {
    const user = userEvent.setup();
    let refused = false;
    const reads = serve(() =>
      refused
        ? jsonResponse(
            { code: "not_found", title: "Not found", status: 404 },
            404,
          )
        : jsonResponse(entry()),
    );
    renderCard(viewWith([entry()]));

    await user.click(screen.getByRole("button", { name: "View Call prep" }));
    await screen.findByText(WORDS);
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByText(WORDS)).toBeNull();
    });

    refused = true;
    const before = reads();
    await user.click(screen.getByRole("button", { name: "View Call prep" }));
    await waitFor(() => {
      expect(reads()).toBeGreaterThan(before);
    });
    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.queryByText(WORDS)).toBeNull();
  });

  it("opens a thread entry only when the page read its activity as readable", () => {
    serve(() => jsonResponse(entry()));
    const projected: components["schemas"]["ConversationMemoryEntry"] = {
      key: ENTRY_ID,
      channel: "note",
      direction: "internal",
      title: "Call prep",
      summary: "A note before the pricing call.",
      occurred_at: "2026-08-28T11:00:00Z",
      occurred_precision: "instant",
      activity_count: 1,
      first_activity_id: ENTRY_ID,
    };
    renderCard(viewWith([entry()], { conversation_memory: [projected] }));
    expect(screen.getByRole("button", { name: "View Call prep" })).toBeTruthy();

    cleanup();
    renderCard(viewWith([], { conversation_memory: [projected] }));
    expect(screen.getByText("Call prep")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "View Call prep" })).toBeNull();
  });
});

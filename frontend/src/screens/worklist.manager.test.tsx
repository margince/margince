// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two interventions a lead makes on somebody else's queue: handing a task
// to another person, and leaving them a note about it.
//
// Both were shipped with stories and no test. A story shows that a control
// draws; neither of these is about drawing. Reassign moves a customer's work
// from one person's day to another's, and coaching writes a notice that arrives
// in a colleague's queue under their own name — so what has to be held is the
// WRITE: which endpoint, and what body. A control that renamed the promise and
// posted nothing looks identical on screen.

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CoachControl, ReassignControl } from "./worklist.manager";
import { WorklistRow } from "./worklist.row";
import { jsonResponse, row } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The roster the pickers read, and the reader themself in it. `LENA` is the
// owner whose queue is open, so she is the person the reassign picker must NOT
// offer — handing work to the person who already holds it is the one move that
// does nothing.
const LENA = "u-lena";
const MINH = "u-minh";

function stubRosterAnd(
  write: (input: Request) => Response | undefined,
  options: { answerMe?: boolean } = {},
) {
  const answerMe = options.answerMe ?? true;
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    const url = String(request ? request.url : input);
    if (request && request.method !== "GET") {
      const answered = write(request);
      if (answered) {
        return answered;
      }
    }
    // Who is reading. The reassign picker falls back to this when no rep is
    // selected, so a stub without it leaves the exclusion comparing against
    // undefined — and the reader is offered their own name.
    if (url.includes("/me")) {
      if (!answerMe) {
        // Pending forever: the holder is never learned.
        return new Promise<Response>(() => {});
      }
      return jsonResponse({ user: { id: LENA, display_name: "Lena Fischer" } });
    }
    if (url.includes("/users")) {
      // The list AND the end of it. `walkRoster` pages until a null cursor, so
      // a body without `page` walks until its own bound and the picker opens
      // empty — which looks exactly like a workspace with nobody in it.
      return jsonResponse({
        data: [
          { id: LENA, display_name: "Lena Fischer" },
          { id: MINH, display_name: "Minh Tran" },
        ],
        page: { next_cursor: null },
      });
    }
    return jsonResponse({ data: [] });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function renderUnderAToastRegion(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The last non-GET this render sent, with its body read off the Request.
//
// openapi-fetch sends a Request object rather than (url, init), so the method
// and the body ride on it — reading a second init argument finds nothing and
// the assertion compares undefined against the write it wanted.
async function wrote(
  fetched: ReturnType<typeof vi.fn>,
): Promise<{ method: string; url: string; body: unknown } | undefined> {
  const calls = fetched.mock.calls;
  for (let at = calls.length - 1; at >= 0; at--) {
    const [input] = calls[at] as [RequestInfo | URL, RequestInit?];
    if (input instanceof Request && input.method !== "GET") {
      return {
        method: input.method,
        url: input.url,
        body: await input.clone().json(),
      };
    }
  }
  return undefined;
}

describe("handing a task to somebody else", () => {
  it("AC-WORKLIST-MGR-03: writes the new assignee to the task the row names", async () => {
    const fetched = stubRosterAnd(() => jsonResponse({}));
    const user = userEvent.setup();
    renderUnderAToastRegion(
      <ReassignControl item={row({ id: "task-1" })} owner={LENA} />,
    );

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );
    await user.click(await screen.findByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "Minh Tran" }));
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.reassignConfirm"],
      }),
    );

    // The task's own endpoint, in the owning module's spelling. This queue adds
    // no writer of its own: it asks the activity to change its assignee.
    const sent = await wrote(fetched);
    expect(sent?.method).toBe("PATCH");
    expect(sent?.url).toContain("/activities/task-1");
    expect(sent?.body).toEqual({ assignee_id: MINH });
    expect(
      await screen.findByText(en["worklist.manager.reassigned"]),
    ).toBeTruthy();
  });

  // The person who already holds the work is not a destination for it.
  it("offers everyone except the person whose queue this is", async () => {
    stubRosterAnd(() => jsonResponse({}));
    const user = userEvent.setup();
    renderUnderAToastRegion(
      <ReassignControl item={row({ id: "task-1" })} owner={LENA} />,
    );

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );

    await user.click(await screen.findByRole("combobox"));
    expect(screen.getByRole("option", { name: "Minh Tran" })).toBeTruthy();
    expect(screen.queryByRole("option", { name: "Lena Fischer" })).toBeNull();
  });

  // A refused handover leaves the work where it was, which looks exactly like a
  // press that did nothing.
  it("says so when the handover is refused", async () => {
    stubRosterAnd(
      () =>
        new Response(JSON.stringify({ title: "Forbidden", status: 403 }), {
          status: 403,
          headers: { "content-type": "application/problem+json" },
        }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion(
      <ReassignControl item={row({ id: "task-1" })} owner={LENA} />,
    );

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );
    await user.click(await screen.findByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "Minh Tran" }));
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.reassignConfirm"],
      }),
    );

    expect(
      await screen.findByText(en["worklist.manager.reassignFailed"]),
    ).toBeTruthy();
  });
});

describe("leaving a note on somebody's queue", () => {
  it("AC-WORKLIST-MGR-03: writes the note to the person it is about", async () => {
    const fetched = stubRosterAnd(() => jsonResponse({}));
    const user = userEvent.setup();
    renderUnderAToastRegion(<CoachControl owner={LENA} />);

    await user.click(
      await screen.findByRole("button", { name: en["worklist.manager.coach"] }),
    );
    await user.type(
      screen.getByLabelText(en["worklist.manager.note"]),
      "Two of these have been waiting a week.",
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.coachConfirm"],
      }),
    );

    const sent = await wrote(fetched);
    expect(sent?.body).toEqual({
      recipient_user_id: LENA,
      kind: "coach_general",
      note: "Two of these have been waiting a week.",
    });
    expect(
      await screen.findByText(en["worklist.manager.coached"]),
    ).toBeTruthy();
  });

  // An empty note is ABSENT rather than an empty string: the coach added none,
  // and the kind's own headline already says what the note would have.
  it("sends no note field where the coach wrote nothing", async () => {
    const fetched = stubRosterAnd(() => jsonResponse({}));
    const user = userEvent.setup();
    renderUnderAToastRegion(<CoachControl owner={LENA} />);

    await user.click(
      await screen.findByRole("button", { name: en["worklist.manager.coach"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.coachConfirm"],
      }),
    );

    const sent = await wrote(fetched);
    expect(sent?.body).toEqual({
      recipient_user_id: LENA,
      kind: "coach_general",
    });
  });
});

// Handing work on is not a manager's verb.
//
// The control used to be gated on a rep being SELECTED, so a reader looking at
// their own queue — where nobody is selected — had no way to pass a task along.
// Dropping that gate is only safe if the picker still refuses the person who
// already holds the task: with no selection the exclusion had nothing to
// compare against, and the reader was offered their own name. A press on it
// posts a reassignment from Lena to Lena, which is a write that changes
// nothing and reads on screen exactly like one that did.
describe("a rep hands on their own task", () => {
  it("offers everyone but the reader, when no rep is selected", async () => {
    stubRosterAnd(() => undefined);
    renderUnderAToastRegion(
      <ReassignControl item={row({ id: "a-1" })} owner="" />,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );
    // The listbox is PORTALLED, so its options are not inside the labelled
    // trigger — reading them through `within(picker)` finds none and the
    // not-offered assertion below passes over an empty list.
    await userEvent.click(
      await screen.findByLabelText(en["worklist.manager.reassignTo"]),
    );
    const offered = (await screen.findAllByRole("option")).map(
      (option) => option.textContent,
    );
    expect(offered).toContain("Minh Tran");
    expect(offered).not.toContain("Lena Fischer");
  });

  // The window between opening the picker and learning who is reading.
  //
  // Nothing on the server refuses a reassignment to the person who already
  // holds the task, so the client filter is the only guard — and a filter with
  // nothing to compare against is not one. A reader quick enough to open the
  // picker before `/me` lands would be offered their own name.
  it("offers nobody until it knows who is reading", async () => {
    // `/me` never answers, which is what makes this the WINDOW rather than a
    // restatement of the test above: with the stub resolving immediately the
    // holder is known before the picker can open, and removing the guard under
    // test changes nothing a click can see.
    const fetched = stubRosterAnd(() => undefined, { answerMe: false });
    const fetchedRoster = () =>
      fetched.mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes("/users"),
      );
    renderUnderAToastRegion(
      <ReassignControl item={row({ id: "a-1" })} owner="" />,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );
    await userEvent.click(
      await screen.findByLabelText(en["worklist.manager.reassignTo"]),
    );
    // The roster has to have LANDED first, or this passes because nothing has
    // rendered yet rather than because the guard held. Minh is in the same
    // answer as Lena, so his absence with the guard in place and his presence
    // without it is what tells the two apart.
    await waitFor(() => {
      expect(fetchedRoster()).toBe(true);
    });
    expect(screen.queryByRole("option", { name: "Minh Tran" })).toBeNull();
    expect(screen.queryByRole("option", { name: "Lena Fischer" })).toBeNull();
  });
});

// And the ROW offers it, which is the change this issue was about.
//
// The two tests above mount ReassignControl directly, so they hold what the
// control does with an empty owner and prove nothing about whether the row
// still hides it. The condition that hid it lived in worklist.row.tsx.
describe("the row a rep is standing on", () => {
  it("offers reassign on the reader's own task", async () => {
    stubRosterAnd(() => undefined);
    renderUnderAToastRegion(
      <WorklistRow
        item={row({ id: "a-1", source: "task" })}
        position={1}
        owner=""
        selected={false}
        onSelect={() => {}}
        onReview={() => {}}
        onOpenEmail={() => {}}
      />,
    );
    expect(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    ).toBeTruthy();
  });
});

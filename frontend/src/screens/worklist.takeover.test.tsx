// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { TeamExceptionsPanel } from "./worklist.exceptions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A lead taking one exception's record for themselves.
//
// The panel answers "what is going wrong" and, until now, could only route to
// whoever answers for it. Taking the work is the intervention a lead reaches
// for when the answer is "nobody is going to get to this", and it writes
// through the module that owns the record rather than through a worklist
// writer that would be a second author of a field five modules already audit.

describe("taking an exception on", () => {
  // The DISPATCH, which is the whole substance of the change. A deal is
  // handed over through the deal's own update and a task through the
  // activity's, and the two spell the field differently — a task is ASSIGNED
  // rather than owned. One table that guessed a single spelling would write a
  // field that does not exist on exactly one subject type.
  it.each([
    ["deal", "/deals/d1", "owner_id"],
    ["lead", "/leads/l1", "owner_id"],
    ["activity", "/activities/a1", "assignee_id"],
  ])("writes a %s through its own module", async (type, path, field) => {
    const fetched = stubTeam({
      type,
      id: path.split("/")[2],
      label: "Something going wrong",
    });
    const user = userEvent.setup();
    renderPanel();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.takeOwnership"],
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.takeOwnershipConfirm"],
      }),
    );

    await waitFor(async () => {
      expect(await patchTo(fetched, path)).toEqual({ [field]: VIEWER });
    });
  });

  // CONFIRMED, because it moves a record out of somebody's day without them
  // pressing anything. A single press that handed the work over would cost two
  // people their sense of what they are carrying on one misplaced click.
  it("asks before it moves anything", async () => {
    const fetched = stubTeam({ type: "deal", id: "d1", label: "Fleet" });
    const user = userEvent.setup();
    renderPanel();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.takeOwnership"],
      }),
    );

    expect(
      screen.getByText(en["worklist.manager.takeOwnershipAsk"]),
    ).toBeTruthy();
    expect(patchCalls(fetched)).toHaveLength(0);
  });

  // A REFUSED handover leaves the record where it was, and the reader has to
  // be told: a silent failure leaves a lead believing they now hold work that
  // is still somebody else's, and they stop watching it.
  it("says so when the handover is refused", async () => {
    stubTeam({ type: "deal", id: "d1", label: "Fleet" }, { refuse: true });
    const user = userEvent.setup();
    renderPanel();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.takeOwnership"],
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.manager.takeOwnershipConfirm"],
      }),
    );

    expect(
      await screen.findByText(en["worklist.manager.takeOwnershipFailed"]),
    ).toBeTruthy();
  });

  // Every row navigates on click, so a button inside one fires BOTH: the
  // handover runs and the page walks away from its own confirmation, which
  // reads as a control that did something unrelated to what it said.
  it("does not also open the owner's queue", async () => {
    stubTeam({ type: "deal", id: "d1", label: "Fleet" });
    const opened = vi.fn();
    const user = userEvent.setup();
    renderPanel(opened);

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.manager.takeOwnership"],
      }),
    );

    expect(opened).not.toHaveBeenCalled();
  });
});

const VIEWER = "00000000-0000-7000-8000-0000000000aa";

// stubTeam answers the panel's read, the viewer probe and the handover write.
//
// Three different addresses, because the panel needs all three and a stub that
// answered them alike would let the assertion below match the exceptions read
// rather than the write it is about.
function stubTeam(
  subject: { type: string; id: string; label: string },
  options: { refuse?: boolean } = {},
) {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    // The METHOD comes off the Request, not off an init bag: openapi-fetch
    // builds a Request and passes it as the first argument, so a stub reading
    // `init.method` sees "GET" for every write and answers the read's body to
    // a PATCH. That is the shape this test exists to assert, so getting it
    // wrong made all four write assertions fail identically and silently.
    const request = input instanceof Request ? input : undefined;
    const url = String(request ? request.url : input);
    if (request?.method === "PATCH") {
      return options.refuse
        ? json({ title: "Forbidden", status: 403 }, 403)
        : json({});
    }
    if (url.includes("/me")) {
      return json({ user: { id: VIEWER, name: "A lead" } });
    }
    if (url.includes("/worklist/exceptions")) {
      return json({
        as_of: "2026-09-05T09:00:00Z",
        truncated: false,
        exceptions: [
          {
            kind: "revenue_at_risk",
            owner: { kind: "user", id: "u1", label: "Lena Fischer" },
            subject,
            since: "2026-08-01T09:00:00Z",
            consequence: "deal_drifts",
            threshold: "at or above the pipeline's median open deal",
          },
        ],
      });
    }
    return json({ data: [] });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// patchCalls are the write attempts, which is what every assertion here is
// about — the reads are setup and would otherwise drown them.
function patchCalls(fetched: ReturnType<typeof vi.fn>) {
  return fetched.mock.calls
    .map((call) => call[0])
    .filter(
      (first): first is Request =>
        first instanceof Request && first.method === "PATCH",
    );
}

// The body is read back off the Request, which consumes it — so each call is
// cloned first. Reading the original would leave the next assertion in the
// same test facing an already-drained stream.
async function patchTo(fetched: ReturnType<typeof vi.fn>, path: string) {
  const request = patchCalls(fetched).find((one) => one.url.includes(path));
  return request ? await request.clone().json() : undefined;
}

function renderPanel(onOwner: (id: string) => void = () => {}) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <TeamExceptionsPanel enabled onOwner={onOwner} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

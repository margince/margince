/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ForecastReview } from "./analytics.forecast.review";
import { jsonResponse } from "./company.fixtures";

// Answering a finding from the forecast review.
//
// One sheet serves every row, so these hold the three ways that shape can go
// wrong: the answer landing on the wrong finding, one finding's draft leaking
// into the next, and a refused save leaving the reader with nothing to read.

type InputCheck = components["schemas"]["InputCheck"];

function check(id: string, label: string): InputCheck {
  return {
    id,
    type: "close_past",
    subject_kind: "deal",
    subject_id: `d-${id}`,
    subject: { type: "deal", id: `d-${id}`, label },
    severity: "high",
    status: "open",
    first_seen_at: "2026-09-01T09:00:00Z",
    last_seen_at: "2026-09-07T09:00:00Z",
  };
}

const FIRST_DEAL = "Fleet retrofit";
const SECOND_DEAL = "Harbor expansion";
const FIRST = check("e-1", FIRST_DEAL);
const SECOND = check("e-2", SECOND_DEAL);

type Posted = { url: string; body: unknown };

// The server as the screen meets it: a resolved finding leaves the list.
// `refuse` answers every save with a problem instead; `hold` keeps each save
// in flight until the test settles it.
function show({
  refuse,
  hold,
}: {
  refuse?: { status: number; detail: string };
  hold?: Promise<void>;
} = {}) {
  const posted: Posted[] = [];
  const reads = { assurance: 0 };
  const resolved = new Set<string>();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (request?.method === "POST") {
        posted.push({ url, body: await request.json() });
        await hold;
        if (refuse) {
          return jsonResponse(
            { title: "Conflict", status: refuse.status, detail: refuse.detail },
            refuse.status,
          );
        }
        const id = /exceptions\/([^/]+)\/resolve/.exec(url)?.[1];
        if (id) {
          resolved.add(id);
        }
        return jsonResponse({});
      }
      if (url.includes("/forecast/assurance/exceptions")) {
        return jsonResponse({
          data: [FIRST, SECOND].filter((item) => !resolved.has(item.id)),
        });
      }
      if (url.includes("/forecast/assurance")) {
        reads.assurance += 1;
        return jsonResponse({ status: "complete", readiness: "needs_review" });
      }
      return jsonResponse({}, 404);
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider initial="en">
        <ForecastReview />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { posted, reads };
}

// The Answer button on the row that names `deal`.
async function answerButton(deal: string): Promise<HTMLElement> {
  const link = await screen.findByRole("link", { name: deal });
  const row = link.closest("tr");
  if (row === null) {
    throw new Error(`no table row holds the link to ${deal}`);
  }
  return within(row).getByRole("button", { name: "Answer" });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("answering a forecast finding", () => {
  it("posts the answer for the row it was opened from, then closes", async () => {
    const user = userEvent.setup();
    const { posted } = show();

    await user.click(await answerButton(SECOND_DEAL));
    const sheet = await screen.findByRole("dialog");
    await user.click(
      within(sheet).getByRole("radio", { name: /Record corrected/ }),
    );
    await user.click(
      within(sheet).getByRole("button", { name: "Save answer" }),
    );

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(posted).toHaveLength(1);
    expect(posted[0]?.url).toContain(
      `/forecast/assurance/exceptions/${SECOND.id}/resolve`,
    );
    expect(posted[0]?.body).toMatchObject({ outcome: "fixed_record" });
  });

  it("opens a different finding blank, whatever was drafted for the last", async () => {
    const user = userEvent.setup();
    show();

    await user.click(await answerButton(FIRST_DEAL));
    const first = await screen.findByRole("dialog");
    await user.click(
      within(first).getByRole("radio", { name: /Value is correct/ }),
    );
    await user.type(
      within(first).getByRole("textbox", { name: /Reason/ }),
      "Buyer confirmed on the call",
    );
    await user.click(within(first).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(await answerButton(SECOND_DEAL));
    const second = await screen.findByRole("dialog");
    for (const radio of within(second).getAllByRole("radio")) {
      expect(radio).toHaveProperty("checked", false);
    }
    // No outcome picked means no reason field at all, so nothing typed for the
    // first finding can be sitting in it.
    expect(
      within(second).queryByRole("textbox", { name: /Reason/ }),
    ).toBeNull();
  });

  it("keeps the sheet open and says why when the server refuses", async () => {
    const user = userEvent.setup();
    show({
      refuse: { status: 409, detail: "This check was already answered." },
    });

    await user.click(await answerButton(FIRST_DEAL));
    const sheet = await screen.findByRole("dialog");
    await user.click(
      within(sheet).getByRole("radio", { name: /Record corrected/ }),
    );
    await user.click(
      within(sheet).getByRole("button", { name: "Save answer" }),
    );

    const alert = await within(sheet).findByRole("alert");
    expect(alert.textContent).toContain("This check was already answered.");
    expect(screen.getByRole("dialog")).toBe(sheet);
  });

  it("leaves the sheet open on another finding when an earlier save lands", async () => {
    const user = userEvent.setup();
    let land = () => {};
    const { posted, reads } = show({
      hold: new Promise<void>((settle) => {
        land = settle;
      }),
    });

    await user.click(await answerButton(FIRST_DEAL));
    const first = await screen.findByRole("dialog");
    await user.click(
      within(first).getByRole("radio", { name: /Record corrected/ }),
    );
    await user.click(
      within(first).getByRole("button", { name: "Save answer" }),
    );
    await user.click(within(first).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(await answerButton(SECOND_DEAL));
    const second = await screen.findByRole("dialog");
    land();
    // The first save has run its course once its row has left the table and
    // the readiness it may move has been read again.
    await waitFor(() => {
      expect(screen.queryByRole("link", { name: FIRST_DEAL })).toBeNull();
      expect(reads.assurance).toBeGreaterThanOrEqual(2);
    });
    expect(posted).toHaveLength(1);
    // And the reader is still answering the second finding.
    await user.click(
      within(second).getByRole("radio", { name: /Remind later/ }),
    );
    expect(screen.getByRole("dialog")).toBe(second);
  });
});

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

import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { suggestion } from "../../testing/dealsuggestion.fixture";
import { meFixture } from "../../testing/mefixture";
import { ReviewActionsProvider } from "../../testing/reviewactions";
import { ForecastReview } from "./analytics.forecast.review";

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
    inputs_changed: false,
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
  findings = [FIRST, SECOND],
  dealActions = ["read", "update"],
  covered = false,
  signalActions = ["read", "update"],
}: {
  refuse?: { status: number; detail: string };
  hold?: Promise<void>;
  findings?: InputCheck[];
  dealActions?: ("read" | "update" | "create")[];
  covered?: boolean;
  signalActions?: ("read" | "update")[];
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
        return jsonResponse(
          url.endsWith("/deals")
            ? { id: "new-support", name: "New support" }
            : {},
        );
      }
      if (url.endsWith("/me"))
        return jsonResponse(
          meFixture({
            allow: {
              deal: dealActions,
              signal: signalActions,
              forecast: ["read", "update"],
            },
          }),
        );
      if (url.includes("/deal-suggestions"))
        return jsonResponse({ data: covered ? [suggestion] : [] });
      if (url.includes("/signals/"))
        return jsonResponse({
          summary: "Annual support request",
          evidence: [],
        });
      if (url.endsWith("/pipelines"))
        return jsonResponse({
          data: [
            {
              id: "pipeline",
              name: "Sales",
              is_default: true,
              stages: [
                { id: "qualified", name: "Qualified", semantic: "open" },
              ],
            },
          ],
        });
      if (url.includes("/deals?"))
        return jsonResponse({
          data: [{ id: "support", name: "Annual support" }],
          page: { has_more: false },
        });
      if (url.includes("/forecast/assurance/exceptions")) {
        return jsonResponse({
          data: findings.filter((item) => !resolved.has(item.id)),
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
        <ReviewActionsProvider>
          <ForecastReview />
        </ReviewActionsProvider>
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

it("requires a chosen deal before filing buying evidence and sends that exact deal", async () => {
  const user = userEvent.setup();
  const finding: InputCheck = {
    ...FIRST,
    type: "unlinked_signal",
    subject_kind: "signal",
    subject_id: "signal-one",
    subject: { type: "company", id: "company-one", label: "Acme" },
    claim: { company_id: "company-one" },
    observed: { occurred_at: "2026-07-01T10:00:00Z", historical: true },
  };
  const { posted } = show({ findings: [finding] });
  await user.click(await answerButton("Acme"));
  const sheet = await screen.findByRole("dialog");
  await user.click(
    await within(sheet).findByRole("radio", { name: "Link to existing deal" }),
  );
  const save = within(sheet).getByRole("button", { name: "Save answer" });
  expect(save.hasAttribute("disabled")).toBe(true);
  const picker = await within(sheet).findByRole("combobox", {
    name: /Existing deal/,
  });
  await waitFor(() => expect(picker.hasAttribute("disabled")).toBe(false));
  await pickOption(user, picker, "Annual support");
  await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
  await user.click(save);
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]?.body).toEqual({
    outcome: "fixed_record",
    deal_id: "support",
  });
});

it("opens the canonical proposal when a buying finding is already covered", async () => {
  const user = userEvent.setup();
  show({
    covered: true,
    dealActions: ["read", "update", "create"],
    findings: [
      {
        ...FIRST,
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: suggestion.company_id, label: "Acme" },
        claim: { company_id: suggestion.company_id },
        observed: { suggestion_id: suggestion.id },
      },
    ],
  });
  await user.click(await answerButton("Acme"));
  const sheet = await screen.findByRole("dialog");
  expect(
    await within(sheet).findByRole("button", { name: "Link to existing deal" }),
  ).toBeTruthy();
  expect(
    within(sheet).queryByRole("button", { name: "Save answer" }),
  ).toBeNull();
  expect(within(sheet).queryByRole("radio")).toBeNull();
});

it("shows an unavailable proposal when its finding has lost the company claim", async () => {
  const user = userEvent.setup();
  show({
    covered: true,
    findings: [
      {
        ...FIRST,
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: suggestion.company_id, label: "Acme" },
        observed: { suggestion_id: suggestion.id },
      },
    ],
  });
  await user.click(await answerButton("Acme"));
  const sheet = await screen.findByRole("dialog");
  expect(
    await within(sheet).findByText(
      "This suggestion is no longer available. Refresh the findings to see its current state.",
    ),
  ).toBeTruthy();
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes(
          "/deal-suggestions",
        ),
      ),
  ).toBe(false);
});

it("does not offer deal linking to a reader without deal update permission", async () => {
  const user = userEvent.setup();
  show({
    dealActions: ["read"],
    findings: [
      {
        ...FIRST,
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: "company-one", label: "Acme" },
        claim: { company_id: "company-one" },
      },
    ],
  });
  await user.click(await answerButton("Acme"));
  const sheet = await screen.findByRole("dialog");
  expect(
    await within(sheet).findByRole("radio", { name: /Not relevant/ }),
  ).toBeTruthy();
  expect(
    within(sheet).queryByRole("radio", { name: "Link to existing deal" }),
  ).toBeNull();
  expect(
    within(sheet).queryByRole("combobox", { name: /Existing deal/ }),
  ).toBeNull();
});

it("keeps a newly qualified deal in the answer and files the signal only after confirmation", async () => {
  const user = userEvent.setup();
  const { posted } = show({
    dealActions: ["read", "create", "update"],
    findings: [
      {
        ...FIRST,
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: "company-one", label: "Acme" },
        claim: { company_id: "company-one" },
      },
    ],
  });
  await user.click(await answerButton("Acme"));
  const answer = await screen.findByRole("dialog");
  await user.click(
    within(answer).getByRole("radio", { name: "Link to existing deal" }),
  );
  await user.click(
    await within(answer).findByRole("button", { name: "New deal" }),
  );
  await user.type(
    await screen.findByRole("textbox", { name: "Deal name" }),
    "New support",
  );
  await user.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]?.url).toMatch(/\/deals$/);
  expect(posted[0]?.body).toMatchObject({
    company_id: "company-one",
    name: "New support",
  });
  const save = within(answer).getByRole("button", {
    name: "Save answer",
  });
  await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
  await user.click(save);
  await waitFor(() => expect(posted).toHaveLength(2));
  expect(posted[1]?.body).toEqual({
    outcome: "fixed_record",
    deal_id: "new-support",
  });
});

it("explains when a signal reader cannot answer the finding", async () => {
  const user = userEvent.setup();
  show({
    signalActions: ["read"],
    findings: [
      {
        ...FIRST,
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: "company-one", label: "Acme" },
        claim: { company_id: "company-one" },
      },
    ],
  });
  await user.click(await answerButton("Acme"));
  const sheet = await screen.findByRole("dialog");
  expect(
    await within(sheet).findByText(
      /You do not have permission for this action/,
    ),
  ).toBeTruthy();
  expect(
    within(sheet).queryByRole("button", { name: "Save answer" }),
  ).toBeNull();
});

// A customer review is answered in the rep's words, and the two answers that
// move the deal go to their own routes. Close as lost asks for the reason and
// no end date: a lost deal has nothing to end.
it("sends a customer review's wait and lost answers to the routes that move the deal", async () => {
  const user = userEvent.setup();
  const review: InputCheck = {
    ...check("e-9", FIRST_DEAL),
    type: "deal_viability",
  };
  const { posted } = show({ findings: [review] });

  await user.click(await answerButton(FIRST_DEAL));
  let sheet = await screen.findByRole("dialog");
  for (const answer of [/customer is back/, /asked to wait/, /Close as lost/]) {
    expect(within(sheet).getByRole("radio", { name: answer })).toBeTruthy();
  }
  await user.click(within(sheet).getByRole("radio", { name: /Close as lost/ }));
  expect(within(sheet).queryByLabelText(/hidden until/i)).toBeNull();
  await user.type(within(sheet).getByRole("textbox"), "Chose another supplier");
  await user.click(within(sheet).getByRole("button", { name: "Save answer" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]?.url).toContain(
    `/forecast/assurance/exceptions/${review.id}/close-lost`,
  );
  expect(posted[0]?.body).toEqual({ lost_reason: "Chose another supplier" });

  await user.click(await answerButton(FIRST_DEAL));
  sheet = await screen.findByRole("dialog");
  await user.click(within(sheet).getByRole("radio", { name: /asked to wait/ }));
  await user.type(
    within(sheet).getByRole("textbox", { name: "Wait until" }),
    "1.12.2026{Enter}",
  );
  await user.click(within(sheet).getByRole("button", { name: "Save answer" }));
  await waitFor(() => expect(posted).toHaveLength(2));
  expect(posted[1]?.url).toContain(
    `/forecast/assurance/exceptions/${review.id}/wait`,
  );
  expect(posted[1]?.body).toEqual({ until: "2026-12-01" });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import * as timezone from "../../format/timezone";
import { en } from "../../i18n/en";
import { SourceHost } from "../../testing/sourcehost";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession as stubSession,
} from "../../testing/story-utils";
import type { NoticeCase } from "./noticecases.logic";
import { NoticeDuty } from "./noticeduty";

const PREVIEW_TOKEN = "a".repeat(64);
const sourceEvidence = {
  reference: { kind: "activity", id: "meeting-source" },
  kind: "meeting",
  title: "Original booking",
  record_type: "activity",
  record_id: "meeting-source",
  occurred_at: "2026-01-01T10:00:00Z",
  occurred_precision: "instant",
  recorded_at: "2026-01-01T10:00:00Z",
};
function stubWithSession(
  routes: Parameters<typeof stubSession>[0],
  allow: Parameters<typeof stubSession>[1],
) {
  stubSession(
    {
      "GET /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ items: [] }),
      "POST /privacy/notice-case-reviews/case-source/source/preview": () =>
        jsonResponse({
          preview_token: PREVIEW_TOKEN,
          rule: "art13",
          notice_required: true,
          due_at: "2026-01-01T10:00:00Z",
          timezone: "Europe/Berlin",
          deadline_is_upper_bound: false,
        }),
      ...routes,
    },
    allow,
  );
}

async function confirmSource(
  user: ReturnType<typeof userEvent.setup>,
  dialog: ReturnType<typeof within>,
) {
  const confirm = dialog.getByRole("button", {
    name: en["notice.source.confirm"],
  });
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);
}

const duty: NoticeCase = {
  id: "case-source",
  contact_id: "contact-source",
  contact_name: "Dana Example",
  rule: "unassessed",
  state: "source_review",
  due_at: null,
  attempts: 0,
  created_at: "2026-01-01T10:00:00Z",
  allowed_routes: [],
};
// The source drawer is a lazy chunk. Loaded once here, so the first case to
// open it, whichever runs first, does not wait out its cold import.
beforeAll(() => import("../voice-knowledge/activitydrawer"));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("confirms an existing source record without retyping its contents", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  stubWithSession(
    {
      "GET /activities/meeting-source": () =>
        jsonResponse({
          id: "meeting-source",
          kind: "meeting",
          subject: "Original booking",
          body: "The original booking request.",
          content_state: "available",
          occurred_at: "2026-01-01T10:00:00Z",
          occurred_precision: "instant",
          version: 1,
        }),
      "GET /privacy/notice-case-reviews/case-source": () => jsonResponse(duty),
      "GET /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ items: [sourceEvidence] }),
      "POST /privacy/notice-case-reviews/case-source/source": (body) => {
        writes.push(body);
        return jsonResponse({ ...duty, state: "open", rule: "art13" });
      },
    },
    {
      privacy_request: ["read", "update"],
      contact: ["read"],
      activity: ["read"],
    },
  );
  render(
    <StoryProviders>
      <SourceHost>
        <NoticeDuty caseId={duty.id} />
      </SourceHost>
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const dialog = within(screen.getByRole("dialog"));
  await user.click(
    await dialog.findByRole("radio", { name: /Original booking/ }),
  );
  await user.click(
    dialog.getByRole("button", { name: en["notice.source.openRecord"] }),
  );
  await screen.findByText("The original booking request.");
  await user.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByText("The original booking request.")).toBeNull(),
  );
  await user.click(
    dialog.getByRole("radio", { name: en["notice.source.answer.met"] }),
  );
  await user.type(
    dialog.getByRole("textbox", { name: en["noticeDuty.obtainedOn"] }),
    "01/01/2026",
  );
  await user.tab();
  expect(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
  ).toHaveValue("");
  await confirmSource(user, dialog);
  await waitFor(() =>
    expect(writes).toEqual([
      expect.objectContaining({
        note: "",
        selected_evidence: { kind: "activity", id: "meeting-source" },
        preview_token: PREVIEW_TOKEN,
      }),
    ]),
  );
});

it("requires a successful preview before confirmation and allows a failed preview to retry", async () => {
  const user = userEvent.setup();
  let refused = true;
  const writes = vi.fn(() => jsonResponse(duty));
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () => jsonResponse(duty),
      "POST /privacy/notice-case-reviews/case-source/source/preview": () =>
        refused
          ? jsonResponse({ title: "Source unavailable", status: 422 }, 422)
          : jsonResponse({
              preview_token: PREVIEW_TOKEN,
              rule: "none",
              notice_required: false,
              due_at: null,
              timezone: "Europe/Berlin",
              deadline_is_upper_bound: false,
            }),
      "POST /privacy/notice-case-reviews/case-source/source": writes,
    },
    { privacy_request: ["read", "update"], contact: ["read"] },
  );
  render(
    <StoryProviders>
      <NoticeDuty caseId={duty.id} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const dialog = within(screen.getByRole("dialog"));
  await user.click(
    dialog.getByRole("radio", { name: en["notice.source.answer.contacted"] }),
  );
  await user.type(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Original letter",
  );
  await dialog.findByRole("alert");
  expect(
    dialog.getByRole("button", { name: en["notice.source.confirm"] }),
  ).toBeDisabled();
  expect(writes).not.toHaveBeenCalled();
  refused = false;
  await user.click(dialog.getByRole("button", { name: en["common.retry"] }));
  await confirmSource(user, dialog);
  await waitFor(() => expect(writes).toHaveBeenCalledTimes(1));
});

it("keeps refused source details hidden when another evidence page arrives", async () => {
  const user = userEvent.setup();
  let pages = 0;
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () => jsonResponse(duty),
      "GET /privacy/notice-case-reviews/case-source/source": () => {
        pages += 1;
        return jsonResponse(
          pages === 1
            ? { items: [sourceEvidence], next_cursor: "next-page" }
            : {
                items: [
                  {
                    ...sourceEvidence,
                    title: "Another appointment",
                    reference: { kind: "activity", id: "another-meeting" },
                    record_id: "another-meeting",
                  },
                ],
              },
        );
      },
      "GET /activities/meeting-source": () =>
        jsonResponse(
          { code: "not_found", status: 404, title: "Not found" },
          404,
        ),
    },
    {
      privacy_request: ["read", "update"],
      contact: ["read"],
      activity: ["read"],
    },
  );
  render(
    <StoryProviders>
      <SourceHost>
        <NoticeDuty caseId={duty.id} />
      </SourceHost>
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const panel = within(screen.getByRole("dialog"));
  await user.click(
    await panel.findByRole("radio", { name: /Original booking/ }),
  );
  await user.click(
    panel.getByRole("button", { name: en["notice.source.openRecord"] }),
  );
  await waitFor(() =>
    expect(panel.queryByRole("radio", { name: /Original booking/ })).toBeNull(),
  );
  // The drawer is a lazy chunk. Escape closes it only once it is drawn.
  await screen.findByText(en["activity.detail.withheldReason"]);
  await user.keyboard("{Escape}");
  await user.click(
    panel.getByRole("button", { name: en["notice.source.moreEvidence"] }),
  );
  await panel.findByRole("radio", { name: /Another appointment/ });
  expect(panel.queryByRole("radio", { name: /Original booking/ })).toBeNull();
});

it("offers source assessment without claiming a duty or offering a notice", async () => {
  vi.spyOn(timezone, "browserZone").mockReturnValue("Asia/Ho_Chi_Minh");
  const user = userEvent.setup();
  const writes: unknown[] = [];
  let current: NoticeCase = duty;
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () =>
        jsonResponse(current),
      "POST /privacy/notice-case-reviews/case-source/source": (body) => {
        writes.push(body);
        current = {
          ...duty,
          rule: "none",
          state: "exempt_with_reason",
          source_kind: "subject_initiated",
          source_review_note: "Incoming original",
          resolution_note:
            "The confirmed source creates no separate notice duty.",
        };
        return jsonResponse(current);
      },
    },
    { privacy_request: ["read", "update"], contact: ["read", "update"] },
  );
  render(
    <StoryProviders>
      <NoticeDuty caseId={duty.id} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const dialog = screen.getByRole("dialog");
  const confirm = within(dialog).getByRole("button", {
    name: en["notice.source.confirm"],
  });
  expect(confirm).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: en["noticeDuty.sendNotice"] }),
  ).not.toBeInTheDocument();
  await user.click(
    within(dialog).getByRole("radio", {
      name: en["notice.source.answer.introduced"],
    }),
  );
  await user.type(
    within(dialog).getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Incoming original",
  );
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(
    within(dialog).getByRole("radio", {
      name: en["notice.source.answer.contacted"],
    }),
  );
  await confirmSource(user, within(dialog));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        kind: "subject_initiated",
        note: "Incoming original",
        obtained_at: null,
        timezone: "Asia/Ho_Chi_Minh",
        met_in_person: false,
        preview_token: PREVIEW_TOKEN,
      },
    ]),
  );
  expect(await screen.findByText(en["notice.rule.none"])).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("keeps an unconfirmed source visible to a read-only privacy officer", async () => {
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () => jsonResponse(duty),
    },
    { privacy_request: ["read"] },
  );
  render(
    <StoryProviders>
      <NoticeDuty caseId={duty.id} />
    </StoryProviders>,
  );
  expect(
    await screen.findByText(en["notice.source.explanation"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: en["notice.source.review"] }),
  ).not.toBeInTheDocument();
});

it("submits the entered acquisition date in the selected calendar timezone", async () => {
  vi.spyOn(timezone, "browserZone").mockReturnValue("Asia/Ho_Chi_Minh");
  const user = userEvent.setup();
  const writes: unknown[] = [];
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () => jsonResponse(duty),
      "POST /privacy/notice-case-reviews/case-source/source": (body) => {
        writes.push(body);
        return jsonResponse({
          ...duty,
          state: "open",
          rule: "art14",
          due_at: "2020-02-28T17:00:00Z",
          source_kind: "referral",
        });
      },
    },
    { privacy_request: ["read", "update"], contact: ["read"] },
  );
  render(
    <StoryProviders>
      <NoticeDuty caseId={duty.id} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const dialog = within(screen.getByRole("dialog"));
  await user.click(
    dialog.getByRole("radio", { name: en["notice.source.answer.introduced"] }),
  );
  await user.type(
    dialog.getByRole("textbox", { name: en["noticeDuty.obtainedOn"] }),
    "31/01/2020",
  );
  await user.tab();
  await user.type(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Original referral record",
  );
  await confirmSource(user, dialog);
  await waitFor(() =>
    expect(writes).toEqual([
      {
        kind: "referral",
        note: "Original referral record",
        obtained_at: "2020-01-30T17:00:00.000Z",
        timezone: "Asia/Ho_Chi_Minh",
        met_in_person: false,
        preview_token: PREVIEW_TOKEN,
      },
    ]),
  );
});

function stubReview(writes: unknown[]) {
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source": () =>
        jsonResponse({
          ...duty,
          acquisition: {
            kind: "unknown_legacy",
            captured_at: "2025-11-03T09:00:00Z",
            captured_by: "system",
          },
        }),
      "POST /privacy/notice-case-reviews/case-source/source": (body) => {
        writes.push(body);
        return jsonResponse({ ...duty, state: "open", rule: "art13" });
      },
    },
    { privacy_request: ["read", "update"] },
  );
}

async function openReview() {
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <NoticeDuty caseId={duty.id} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  return { user, dialog: within(screen.getByRole("dialog")) };
}

it("says why it asks and what is on record before the question", async () => {
  stubReview([]);
  const { dialog } = await openReview();

  expect(
    await dialog.findByText(en["notice.source.whyUnknown"]),
  ).toBeInTheDocument();
  expect(dialog.getByText(en["notice.source.onRecord"])).toBeInTheDocument();
  expect(
    dialog.getByRole("group", { name: en["notice.source.question"] }),
  ).toBeInTheDocument();
});

it("records a face-to-face meeting with its day, which supports follow-up mail and owes no notice", async () => {
  vi.spyOn(timezone, "browserZone").mockReturnValue("Europe/Berlin");
  const writes: unknown[] = [];
  stubReview(writes);
  const { user, dialog } = await openReview();

  await user.click(
    dialog.getByRole("radio", {
      name: en["notice.source.answer.met"],
    }),
  );
  expect(dialog.getByText(en["notice.source.metEffect"])).toBeInTheDocument();
  await user.type(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Card from the trade fair",
  );
  const confirm = dialog.getByRole("button", {
    name: en["notice.source.confirm"],
  });
  expect(confirm).toBeDisabled();
  await user.type(
    dialog.getByRole("textbox", { name: en["noticeDuty.obtainedOn"] }),
    "31/01/2020",
  );
  await user.tab();
  await confirmSource(user, dialog);

  await waitFor(() =>
    expect(writes).toEqual([
      {
        kind: "in_person_permission",
        note: "Card from the trade fair",
        obtained_at: "2020-01-30T23:00:00.000Z",
        timezone: "Europe/Berlin",
        met_in_person: true,
        preview_token: PREVIEW_TOKEN,
      },
    ]),
  );
});

it("offers the remaining kinds behind Another source", async () => {
  vi.spyOn(timezone, "browserZone").mockReturnValue("Europe/Berlin");
  const writes: unknown[] = [];
  stubReview(writes);
  const { user, dialog } = await openReview();

  await user.click(
    dialog.getByRole("radio", { name: en["notice.source.answer.other"] }),
  );
  const second = dialog.getByRole("group", {
    name: en["notice.source.otherQuestion"],
  });
  expect(within(second).getAllByRole("radio")).toHaveLength(6);
  await user.click(
    within(second).getByRole("radio", {
      name: en["notice.acq.purchasedOrImported"],
    }),
  );
  await user.type(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Bought list, spring fair",
  );
  await confirmSource(user, dialog);

  await waitFor(() =>
    expect(writes).toEqual([
      expect.objectContaining({ kind: "purchased_or_imported" }),
    ]),
  );
});

it("leaves the source unconfirmed and saves nothing when the reviewer is not sure", async () => {
  const writes: unknown[] = [];
  stubReview(writes);
  const { user, dialog } = await openReview();

  await user.click(
    dialog.getByRole("radio", {
      name: new RegExp(`^${en["notice.source.answer.unsure"]}`),
    }),
  );
  expect(
    dialog.queryByRole("textbox", { name: en["notice.source.evidence"] }),
  ).not.toBeInTheDocument();
  await user.click(
    dialog.getByRole("button", { name: en["notice.source.leave"] }),
  );

  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(writes).toEqual([]);
});

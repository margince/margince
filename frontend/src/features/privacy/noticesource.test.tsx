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
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import * as timezone from "../../format/timezone";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import type { NoticeCase } from "./noticecases.logic";
import { NoticeDuty } from "./noticeduty";

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
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("offers source assessment without claiming a duty or offering a notice", async () => {
  vi.spyOn(timezone, "viewerZone").mockReturnValue("Asia/Ho_Chi_Minh");
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
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: en["noticeDuty.obtainedAs"] }),
    en["notice.acq.referral"],
  );
  await user.type(
    within(dialog).getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Incoming original",
  );
  expect(confirm).toBeEnabled();
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: en["noticeDuty.obtainedAs"] }),
    en["notice.acq.subjectInitiated"],
  );
  await user.click(confirm);
  await waitFor(() =>
    expect(writes).toEqual([
      {
        kind: "subject_initiated",
        note: "Incoming original",
        obtained_at: null,
        timezone: "Asia/Ho_Chi_Minh",
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
  vi.spyOn(timezone, "viewerZone").mockReturnValue("Asia/Ho_Chi_Minh");
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
  await pickOption(
    user,
    dialog.getByRole("combobox", { name: en["noticeDuty.obtainedAs"] }),
    en["notice.acq.referral"],
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
  await user.click(
    dialog.getByRole("button", { name: en["notice.source.confirm"] }),
  );
  await waitFor(() =>
    expect(writes).toEqual([
      {
        kind: "referral",
        note: "Original referral record",
        obtained_at: "2020-01-30T17:00:00.000Z",
        timezone: "Asia/Ho_Chi_Minh",
      },
    ]),
  );
});

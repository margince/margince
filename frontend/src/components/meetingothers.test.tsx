/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StoryProviders } from "../testing/story-utils";
import { MeetingOthers } from "./meetingothers";

afterEach(cleanup);

const others = [
  { kind: "colleague" as const, name: "Sam Okafor" },
  { kind: "address" as const, name: "lena@prospect.test" },
];

function text(ui: React.ReactElement) {
  return render(<StoryProviders>{ui}</StoryProviders>).container.textContent;
}

it("continues the contacts' list, then says how many more there are", () => {
  expect(text(<MeetingOthers others={others} more={3} afterContacts />)).toBe(
    ", Sam Okafor, lena@prospect.test and 3 more",
  );
});

it("starts the list when no contact is named", () => {
  expect(text(<MeetingOthers others={others} afterContacts={false} />)).toBe(
    "Sam Okafor, lena@prospect.test",
  );
});

it("follows the contacts with the count alone, and draws nothing when there is nothing to add", () => {
  expect(text(<MeetingOthers more={1} afterContacts />)).toBe(" and 1 more");
  expect(text(<MeetingOthers afterContacts />)).toBe("");
});

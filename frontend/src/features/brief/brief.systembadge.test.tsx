/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { en } from "../../i18n/en";
import {
  readingsDay,
  taskRow,
  wholeMeetings,
} from "../../testing/brief.fixtures";
import { render } from "../../testing/brief.testkit";
import { BriefFeed } from "./brief.feed";

afterEach(cleanup);

const rows = [taskRow("task-0", "Call Weber")];

it("counts the system items beside Needs you and opens the System view", () => {
  const day = readingsDay({}, rows, [
    wholeMeetings(0),
    {
      category: "system",
      considered: 3,
      total: 3,
      shown: 0,
      more_available: false,
    },
  ]);
  render(<BriefFeed day={day} state="ready" />);
  const link = screen.getByRole("link", {
    name: en["brief.agenda.system_other"].replace("{count}", "3"),
  });
  expect(link.getAttribute("href")).toContain(
    "#/home?queue=1&queue_view=system",
  );
});

it("names no system items on a day without any", () => {
  render(<BriefFeed day={readingsDay({}, rows)} state="ready" />);
  expect(
    screen.queryByText(en["brief.agenda.system_other"].replace("{count}", "0")),
  ).toBeNull();
  expect(screen.queryByText(/system issue/)).toBeNull();
});

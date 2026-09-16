/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BriefScreen } from "./brief";
import { readingsDay, taskRow } from "./brief.fixtures";
import { jsonResponse, render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("keeps the summary visible and puts informational updates after the priorities", async () => {
  const task = taskRow("task-one", "Send the promised comparison");
  const notice = {
    ...taskRow("notice-one", "Northstar"),
    source: "notice" as const,
    category: "system" as const,
    level: 5,
    urgent: false,
    detail: "Obsolete delivery text",
    notice_origin: {
      event_id: "stage-event",
      actor_type: "human",
      actor_id: "human:colleague",
      actor_name: "Dana Weiss",
      occurred_at: "2026-09-07T10:00:00Z",
      stage_change: { from_name: "Qualified", to_name: "Won" },
    },
    actions: ["acknowledge" as const],
  };
  stubApi({
    "GET /worklist": () =>
      jsonResponse({
        ...readingsDay({}, [task, notice]),
        focus: { items: [task], total: 1, urgent_remaining: 0 },
      }),
  });
  const { container } = render(<BriefScreen />);
  await screen.findAllByText("Send the promised comparison");
  const summary = await screen.findByRole("region", {
    name: "Your morning, in five readings",
  });
  expect(summary.closest("details")).toBeNull();
  expect(summary.closest(".brief-overview")).toBeTruthy();
  expect(container.querySelector("#brief-today")?.textContent).not.toContain(
    "Northstar",
  );
  expect(
    container.querySelector(".brief-followthrough")?.textContent,
  ).toContain("Northstar");
  expect(container.textContent).toContain(
    "Qualified → Won · Changed by: Dana Weiss",
  );
  expect(container.textContent).not.toContain("Obsolete delivery text");
  expect(container.textContent).toContain("Agenda updated");
  expect(container.textContent).not.toContain("Some work may be missing");
});

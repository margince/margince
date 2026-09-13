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

it("keeps the work summary open and puts updates beside actionable work", async () => {
  const task = taskRow("task-one", "Send the promised comparison");
  const notice = {
    ...taskRow("notice-one", "Northstar changed stage"),
    source: "notice" as const,
    category: "system" as const,
    level: 6,
    urgent: false,
    detail: "Northstar moved to a new pipeline stage.",
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
  await screen.findByText("Send the promised comparison");
  const summary = await screen.findByText("Work summary");
  expect(summary.closest("details")).toBeNull();
  expect(summary.closest(".brief-rail")).toBeTruthy();
  expect(container.querySelector("#brief-today")?.textContent).not.toContain(
    "Northstar changed stage",
  );
  expect(container.querySelector(".brief-rail")?.textContent).toContain(
    "Northstar changed stage",
  );
  expect(container.textContent).toContain("Agenda updated");
  expect(container.textContent).not.toContain("Some work may be missing");
});

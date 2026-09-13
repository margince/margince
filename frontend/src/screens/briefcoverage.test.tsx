/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { readingsDay } from "./brief.fixtures";
import { render } from "./brief.testkit";
import { BriefCoverage } from "./briefcoverage";

afterEach(cleanup);
it("renders nothing for a complete read", () => {
  const { container } = render(<BriefCoverage day={readingsDay({}, [])} />);
  expect(container.innerHTML).toBe("");
});
it("names the affected source without technical limit prose", () => {
  const day = readingsDay({}, []);
  day.reach = [
    { source: "notice", considered: 8, shown: 8, more_available: true },
  ];
  render(<BriefCoverage day={day} />);
  expect(screen.getByText(en["brief.coverage.summary"])).toBeTruthy();
  expect(screen.getByText(/More Notices may be available/)).toBeTruthy();
  expect(document.body.textContent).not.toMatch(
    /Read to a limit|A notice for you|8 shown/,
  );
});
it("offers refresh for a failed source and names it", async () => {
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  const retry = vi.fn();
  render(<BriefCoverage day={day} onRetry={retry} />);
  await userEvent.click(screen.getByText(en["brief.coverage.summary"]));
  await userEvent.click(
    screen.getByRole("button", { name: en["brief.coverage.retry"] }),
  );
  expect(retry).toHaveBeenCalledOnce();
  expect(document.body.textContent).toContain("Tasks");
});

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
it("does not turn a bounded scan or withheld source into a generic alarm", () => {
  const day = readingsDay({}, []);
  day.reach = [
    { source: "notice", considered: 8, shown: 8, more_available: true },
  ];
  day.sources_unavailable = [{ source: "dsr", reason: "withheld" }];
  const { container } = render(<BriefCoverage day={day} />);
  expect(container.innerHTML).toBe("");
});
it("offers refresh for a failed source and names it", async () => {
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  const retry = vi.fn();
  render(<BriefCoverage day={day} onRetry={retry} />);
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("button", { name: en["brief.coverage.retry"] }),
  );
  expect(retry).toHaveBeenCalledOnce();
  expect(document.body.textContent).toContain("Tasks");
});

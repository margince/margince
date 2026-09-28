/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { ranked, readingsDay } from "./brief.fixtures";
import { render } from "./brief.testkit";
import { BriefCoverage } from "./briefcoverage";

afterEach(cleanup);
it("renders nothing for a complete read", () => {
  const { container } = render(<BriefCoverage day={readingsDay({}, [])} />);
  expect(container.innerHTML).toBe("");
});
it("does not turn a bounded scan into a missing source", () => {
  const day = readingsDay({}, []);
  day.reach = [
    { source: "notice", considered: 8, shown: 8, more_available: true },
  ];
  const { container } = render(<BriefCoverage day={day} />);
  expect(container.innerHTML).toBe("");
});
// A failed read can arrive on a refetch and brings a retry, so it is spoken. A
// grant that withheld a source is true on every mount, and speaking it would
// say "part of your day is hidden" aloud forever.
it("speaks a failed source and stays silent about a withheld one", () => {
  const failing = readingsDay({}, []);
  failing.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  const { container: spoken } = render(<BriefCoverage day={failing} />);
  expect(spoken.querySelector('[role="status"]')).not.toBeNull();
  cleanup();

  const withheld = readingsDay({}, []);
  withheld.sources_unavailable = [{ source: "dsr", reason: "withheld" }];
  const { container: silent } = render(
    <BriefCoverage
      day={withheld}
      run={{ ...ranked, factors_omitted: ["warmth"] }}
    />,
  );
  expect(silent.querySelector('[role="status"]')).toBeNull();
});
// The mixed day is the one that decides it. A failure alongside a standing
// withholding must announce only the failure, or the grant gets spoken after
// all, on the back of an unrelated retry.
it("announces the failure alone when a withheld line sits beside it", () => {
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
    { source: "dsr", reason: "withheld" },
  ];
  const { container } = render(
    <BriefCoverage
      day={day}
      run={{ ...ranked, factors_omitted: ["warmth"] }}
      onRetry={vi.fn()}
    />,
  );
  const spoken = container.querySelector('[role="status"]');
  expect(spoken?.textContent).toContain("Tasks");
  expect(spoken?.textContent).not.toContain("Privacy requests");
  expect(spoken?.textContent).not.toContain(en["brief.factor.warmth"]);
  // Still on the page, just not announced.
  expect(document.body.textContent).toContain("Privacy requests");
  expect(document.body.textContent).toContain(en["brief.factor.warmth"]);
});
it("names a withheld source without dressing it as a fault", () => {
  const day = readingsDay({}, []);
  day.sources_unavailable = [{ source: "dsr", reason: "withheld" }];
  render(<BriefCoverage day={day} onRetry={vi.fn()} />);
  expect(document.body.textContent).toContain("Privacy requests");
  expect(
    screen.queryByRole("button", { name: en["brief.coverage.retry"] }),
  ).toBeNull();
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
it("says the order does not account for a factor the run could not read", () => {
  render(
    <BriefCoverage
      day={readingsDay({}, [])}
      run={{ ...ranked, factors_omitted: ["warmth"] }}
    />,
  );
  expect(document.body.textContent).toContain(en["brief.factor.warmth"]);
  // The frame from the catalog rather than a phrase typed here, so a reworded
  // caveat fails on its own line instead of on a stale copy of itself.
  expect(document.body.textContent).toContain(
    en["brief.order.withheld"].split("{factors}")[0].trim(),
  );
});
it("stays silent for a run that weighed every factor", () => {
  const { container } = render(
    <BriefCoverage day={readingsDay({}, [])} run={ranked} />,
  );
  expect(container.innerHTML).toBe("");
});
it("offers no retry for a factor a grant withheld", () => {
  render(
    <BriefCoverage
      day={readingsDay({}, [])}
      run={{ ...ranked, factors_omitted: ["warmth"] }}
      onRetry={vi.fn()}
    />,
  );
  expect(
    screen.queryByRole("button", { name: en["brief.coverage.retry"] }),
  ).toBeNull();
});
it("draws a failed source and a withheld factor together", () => {
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  render(
    <BriefCoverage
      day={day}
      run={{ ...ranked, factors_omitted: ["warmth"] }}
      onRetry={vi.fn()}
    />,
  );
  expect(document.body.textContent).toContain("Tasks");
  expect(document.body.textContent).toContain(en["brief.factor.warmth"]);
  expect(
    screen.getByRole("button", { name: en["brief.coverage.retry"] }),
  ).toBeTruthy();
});

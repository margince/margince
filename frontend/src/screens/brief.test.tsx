/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import { readingsDay, taskRow } from "./brief.fixtures";
import {
  jsonResponse,
  pendingPage,
  proposal,
  render,
  stubApi,
  writes,
} from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("shows one daily agenda, without duplicate tasks, risks or an empty approvals panel", async () => {
  stubApi({
    "GET /worklist": () =>
      jsonResponse(readingsDay({}, [taskRow("t", "Follow up with Weber")])),
  });
  const { container } = render(<BriefScreen />);
  await screen.findAllByText("Follow up with Weber");
  expect(container.querySelectorAll(".worklist-row-title")).toHaveLength(1);
  expect(container.querySelector("#brief-tasks")).toBeNull();
  expect(container.querySelector("#brief-decisions")).toBeNull();
  expect(container.querySelector("#brief-watch")).toBeNull();
  expect(container.querySelector("time")).toBeTruthy();
});

it("continues the full queue in a Brief drawer without losing the first page or repeating a boundary row", async () => {
  const first = taskRow("t1", "Call Weber");
  const second = taskRow("t2", "Review Nordwind");
  stubApi({
    "GET /worklist": (_body, query) =>
      jsonResponse(
        !query.has("cursor")
          ? { ...readingsDay({}, [first]), next_cursor: "page-two" }
          : readingsDay({}, [first, second]),
      ),
  });
  render(<BriefScreen />);
  await screen.findAllByText("Call Weber");
  await userEvent.click(
    screen.getByRole("button", { name: new RegExp(en["brief.queue.show"]) }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: en["worklist.more"] }),
  );
  await screen.findByText("Review Nordwind");
  expect(
    document
      .querySelector("[role=dialog]")
      ?.querySelectorAll(".worklist-row-title"),
  ).toHaveLength(2);
  expect(
    screen.queryByRole("button", { name: en["worklist.more"] }),
  ).toBeNull();
});

it("an approval-only morning stays actionable, and reviewing it sends no decision", async () => {
  const approval = proposal("a1", "Email the buyer");
  const row = {
    ...taskRow("a1", "Email the buyer"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: ["decide" as const],
  };
  const calls = stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [row])),
    "GET /approvals/a1": () => jsonResponse(approval),
  });
  render(<BriefScreen />);
  await userEvent.click(
    await screen.findByRole("button", { name: en["worklist.verb.decide"] }),
  );
  await screen.findByRole("dialog");
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: en["brief.approval.email"] }),
    ).toBeTruthy(),
  );
  expect(writes(calls)).toEqual([]);
  expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
});

it("opens a bundle's effects together from the ranked row", async () => {
  const one = proposal("a1", "First effect", { bundle_id: "bundle" });
  const two = proposal("a2", "Second effect", { bundle_id: "bundle" });
  const row = {
    ...taskRow("a1", "Review the proposal bundle"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: ["decide" as const],
  };
  const calls = stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [row])),
    "GET /approvals/a1": () => jsonResponse(one),
    "GET /approvals": () => pendingPage([one, two], new Set()),
  });
  render(<BriefScreen />);
  await userEvent.click(
    await screen.findByRole("button", { name: en["worklist.verb.decide"] }),
  );
  await userEvent.click(await screen.findByText("Show the 2 items"));
  expect(screen.getByText("Second effect")).toBeTruthy();
  expect(writes(calls)).toEqual([]);
});

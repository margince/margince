/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { LocaleProvider, useLocale, useT } from "../i18n";
import { en } from "../i18n/en";
import { leadTodoRows } from "./leadtoday";
import { TodayPanel } from "./record360";

// leadTodoRows split out of leads.tsx (leads.test.tsx is at its own line
// ceiling) so the "what needs a contact" rows, and the verb each row hands
// the reader to resolve it, can be tested on their own.

afterEach(cleanup);

type Lead = components["schemas"]["Lead"];

// A COMPLETE Lead, not a cast one: every field the fixture omits is one this
// suite is trusting the contract's own optionality for.
const BASE: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  status: "contacted",
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

function Rows({
  lead,
  onOpenTasks = () => {},
}: Readonly<{
  lead: Lead;
  onOpenTasks?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <TodayPanel onOpenTasks={onOpenTasks} tasksLabel={t("today.workQueue")}>
      {leadTodoRows(
        lead,
        t,
        locale,
        // The reader's own zone, read the way every screen reads it: this
        // case is about what the rows SAY, and a hand-picked zone here would
        // be a second answer to a question the one module already answers.
        viewerZone(),
        onOpenTasks,
      )}
    </TodayPanel>
  );
}

function show(props: Parameters<typeof Rows>[0]) {
  render(
    <LocaleProvider initial="en">
      <Rows {...props} />
    </LocaleProvider>,
  );
}

describe("what needs a contact on a lead, and the verb that resolves it", () => {
  it("draws no rows at all on a closed lead", () => {
    // Directly on the function: closed is the one state that draws nothing,
    // and a pure function is the shortest path to proving that.
    expect(
      leadTodoRows(
        {
          ...BASE,
          archived_at: "2026-07-01T00:00:00Z",
          first_response_at: null,
        },
        (key: string) => key,
        "en",
        // The reader's own zone, read the way every screen reads it: this
        // case is about what the rows SAY, and a hand-picked zone here would
        // be a second answer to a question the one module already answers.
        viewerZone(),
        vi.fn(),
      ),
    ).toEqual([]);
  });

  it.each(["manual", "import", "webform"])(
    "a %s lead with no first response creates no task",
    (source) => {
      show({
        lead: {
          ...BASE,
          source,
          first_response_at: null,
          sla_state: "breached",
          sla_deadline_at: "2026-06-02T08:00:00Z",
        },
      });
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
      expect(screen.queryByText("First response due")).toBeNull();
    },
  );

  it("the Next task row's verb opens the same queue the panel head does", async () => {
    const onOpenTasks = vi.fn();
    show({
      lead: {
        ...BASE,
        first_response_at: "2026-06-02T08:00:00Z",
        next_task_subject: "Send the proposal",
      },
      onOpenTasks,
    });
    fireEvent.click(
      await screen.findByRole("button", { name: en["lead.today.openTasks"] }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: en["today.workQueue"] }),
    );
    expect(onOpenTasks).toHaveBeenCalledTimes(2);
  });

  it("says nothing is next once no open task is linked", () => {
    show({ lead: { ...BASE, first_response_at: "2026-06-02T08:00:00Z" } });
    expect(
      screen.queryByRole("button", { name: en["lead.today.openTasks"] }),
    ).toBeNull();
  });
});

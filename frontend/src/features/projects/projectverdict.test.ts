// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { translate, translatePlural } from "../../i18n";
import { project } from "../../testing/projects.fixtures";
import { deliveringProject360 } from "./projectstanding.fixtures";
import { projectVerdict } from "./projectstanding.words";

const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const ctx = {
  t,
  plural: (
    base: Parameters<typeof translatePlural>[1],
    count: number,
    values?: Record<string, string>,
  ) => translatePlural("en", base, count, values),
  locale: "en",
  zone: UTC_ZONE,
} as const;

// Nobody assessed these projects, so the card reads the dates and the tasks.
function unassessed(over: Parameters<typeof project>[0], overdue = false) {
  const base = deliveringProject360();
  const commitments = base.commitments ?? {
    data: [],
    page: { has_more: false },
  };
  return deliveringProject360({
    health: {},
    project: project({ ...base.project, ...over }),
    commitments: {
      ...commitments,
      data: commitments.data.map((row) => ({ ...row, overdue })),
    },
  });
}

describe("an unassessed project's verdict", () => {
  it("is on schedule while running with nothing overdue", () => {
    expect(projectVerdict(unassessed({}), ctx)).toEqual({
      word: t("project.verdict.onSchedule"),
      sentence: t("project.verdict.dayOf", { day: "166", total: "275" }),
      tone: "success",
    });
  });

  it("is behind with an overdue task, and names the first", () => {
    expect(projectVerdict(unassessed({}, true), ctx)).toEqual({
      word: t("project.verdict.behind"),
      sentence: "1 task is overdue: Send the kickoff agenda.",
      tone: "warning",
    });
  });

  it("is overrunning once the target end has passed", () => {
    expect(
      projectVerdict(unassessed({ target_end_date: "2026-08-01" }), ctx),
    ).toMatchObject({ word: t("project.verdict.overrunning"), tone: "danger" });
  });

  it("is wrapping up in its last stretch", () => {
    expect(
      projectVerdict(unassessed({ target_end_date: "2026-08-20" }), ctx),
    ).toEqual({
      word: t("project.verdict.wrappingUp"),
      sentence: "7 days are left, and no task is overdue.",
      tone: "success",
    });
  });

  it("is not started before its start day", () => {
    expect(
      projectVerdict(unassessed({ started_at: "2026-09-01" }), ctx),
    ).toMatchObject({ word: t("project.standing.notStarted") });
  });

  it("makes no call without dates", () => {
    expect(projectVerdict(unassessed({ started_at: null }), ctx)).toMatchObject(
      { word: t("standing.notAssessed") },
    );
  });
});

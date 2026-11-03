// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { translate } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { committeeTeam } from "./dealcommitteeteam";

type RecordAssignment = components["schemas"]["RecordAssignment"];
type DealCoverage = components["schemas"]["DealCoverage"];

const t = (key: MessageKey, vars?: Record<string, string>) =>
  translate("en", key, vars);

function held(
  subject: string,
  name: string,
  role: string,
  kind: RecordAssignment["subject_kind"] = "user",
): RecordAssignment {
  return {
    id: `a-${subject}-${role}`,
    record_type: "deal",
    record_id: "d1",
    subject_kind: kind,
    subject_id: subject,
    subject_name: name,
    subject_inactive: false,
    role_id: `r-${role}`,
    role_key: role,
    role_label: role,
    role_active: true,
    version: 1,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  };
}

const carried = (
  userId: string,
  ourSide: DealCoverage["our_side"],
): DealCoverage => ({
  deal_id: "d1",
  stakeholders: [],
  our_side: ourSide,
  risks: [
    {
      kind: "single_threaded_ours",
      summary: "one carries it",
      user_ids: [userId],
    },
  ],
  sections_omitted: [],
});

describe("our side of the committee", () => {
  it("gives the owner one row with every job they hold", () => {
    const team = committeeTeam({
      owner: { id: "u1", name: "Lena Fischer" },
      assignments: [
        held("u1", "Lena Fischer", "Sales owner"),
        held("u2", "Tomás Reyes", "Delivery owner"),
        held("u2", "Tomás Reyes", "Implementation lead"),
      ],
      t,
      locale: "en",
    });
    expect(team.map((row) => [row.name, row.jobs])).toEqual([
      ["Lena Fischer", "Owner, Sales owner"],
      ["Tomás Reyes", "Delivery owner, Implementation lead"],
    ]);
  });

  it("names a built-in job in the reader's language and a relabelled one as typed", () => {
    const de = (key: MessageKey, vars?: Record<string, string>) =>
      translate("de", key, vars);
    const team = committeeTeam({
      assignments: [
        {
          ...held("u1", "Lena Fischer", "Account manager"),
          role_label_key: "recordRoles.builtin.account_manager",
        },
        held("u1", "Lena Fischer", "Presales lead"),
      ],
      t: de,
      locale: "de",
    });
    expect(team[0]?.jobs).toBe(
      `${translate("de", "recordRoles.builtin.account_manager")}, Presales lead`,
    );
  });

  it("keeps a team apart from a colleague and says it is a team", () => {
    const team = committeeTeam({
      assignments: [held("x", "Delivery", "Support", "team")],
      t,
      locale: "en",
    });
    expect(team[0]?.name).toBe(`Delivery ${t("assignments.teamSuffix")}`);
  });

  it("lists the colleague the finding names even when they hold no job", () => {
    const team = committeeTeam({
      owner: { id: "u1", name: "Lena Fischer" },
      assignments: [],
      coverage: carried("u9", [
        {
          user_id: "u9",
          display_name: "Ana Lima",
          strength_bucket: "strong",
          interactions_90d: 24,
        },
      ]),
      t,
      locale: "en",
    });
    expect(team.map((row) => [row.name, row.jobs, row.carries])).toEqual([
      ["Lena Fischer", "Owner", false],
      ["Ana Lima", "", true],
    ]);
  });

  it("invents no row for a colleague the read did not name", () => {
    const team = committeeTeam({
      assignments: [],
      coverage: carried("u9", []),
      t,
      locale: "en",
    });
    expect(team).toEqual([]);
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { ListQuery } from "../../components/listquery";
import { UNNARROWED_LIST_QUERY } from "../../testing/listquery.fixture";
import {
  leadStatusReportFilters,
  terminalCountsRefusedBecause,
  terminalCountsWithheldBecause,
  terminalLeadsQueryParams,
} from "./leadboard.queries";

function narrowed(filters: Record<string, string>, q = ""): ListQuery {
  return { ...UNNARROWED_LIST_QUERY, q, filters };
}

describe("the terminal columns' counts", () => {
  it("are counted under every dial the report can filter by", () => {
    const query = narrowed({
      status: "promoted",
      owner_id: "u-me",
      source: "webform",
      origin: "event",
    });
    expect(terminalCountsWithheldBecause(query)).toBeUndefined();
    expect(leadStatusReportFilters(query)).toEqual({
      status: "promoted",
      owner_id: "u-me",
      source: "webform",
      origin: "event",
    });
  });

  it("ask the report for leads with no origin when the dial says unset", () => {
    const query = narrowed({ origin: "unset" });
    expect(terminalCountsWithheldBecause(query)).toBeUndefined();
    expect(leadStatusReportFilters(query)).toEqual({ origin: null });
  });

  it("are withheld for a whole connector, and counted for one account", () => {
    expect(
      terminalCountsWithheldBecause(narrowed({ source: "connector:apollo" })),
    ).toBe("lead.boardCountsNoConnectorSource");
    expect(
      terminalCountsWithheldBecause(
        narrowed({ source: "connector:apollo:a-1" }),
      ),
    ).toBeUndefined();
  });

  it("send no filter for an unnarrowed board", () => {
    expect(leadStatusReportFilters(UNNARROWED_LIST_QUERY)).toEqual({});
  });

  it("stay counted when the list widens to archived rows or sorts", () => {
    expect(
      terminalCountsWithheldBecause({
        ...UNNARROWED_LIST_QUERY,
        includeArchived: true,
        sort: "-score",
      }),
    ).toBeUndefined();
  });

  it.each([
    [narrowed({}, "nordwind"), "lead.boardCountsNoSearch"],
    [narrowed({ tag_id: "t-1,t-2" }), "lead.boardCountsNoTagFilter"],
    [narrowed({ owner_team_id: "team-1" }), "lead.boardCountsNoTeamFilter"],
    [narrowed({ unassigned: "true" }), "lead.boardCountsNoTeamFilter"],
    [narrowed({ min_score: "80" }), "lead.boardCountsNoScoreFilter"],
    [narrowed({ sla_state: "breached" }), "lead.boardCountsNoResponseFilter"],
    [narrowed({ industry: "software" }), "lead.boardCountsNoCompanyFilter"],
    [narrowed({ country: "DE" }), "lead.boardCountsNoCompanyFilter"],
    [narrowed({ size_band: "51-200" }), "lead.boardCountsNoCompanyFilter"],
    [narrowed({ partner_company_id: "c-9" }), "lead.boardCountsNoOtherFilter"],
  ])("are withheld under a dial the report cannot apply", (query, reason) => {
    expect(terminalCountsWithheldBecause(query)).toBe(reason);
  });

  it("name the uncountable dial even beside countable ones", () => {
    expect(
      terminalCountsWithheldBecause(
        narrowed({ owner_id: "u-me", min_score: "60" }),
      ),
    ).toBe("lead.boardCountsNoScoreFilter");
  });

  it("ignore a tag mode with no tag to combine", () => {
    expect(
      terminalCountsWithheldBecause(narrowed({ tag_mode: "none" })),
    ).toBeUndefined();
  });

  it("read a 403 from the owner gate as refused, and nothing else", () => {
    expect(terminalCountsRefusedBecause(null)).toBe(
      "lead.boardCountsOwnerNotMeasurable",
    );
    expect(terminalCountsRefusedBecause(undefined)).toBeUndefined();
    expect(terminalCountsRefusedBecause(new Map())).toBeUndefined();
  });
});

describe("a terminal column's cards", () => {
  it("are read under the list's dials, held to the column's status", () => {
    const params = terminalLeadsQueryParams(
      narrowed({ owner_id: "u-me", min_score: "80", tag_id: "t-1" }, "acme"),
      "disqualified",
    );
    expect(params).toMatchObject({
      q: "acme",
      owner_id: "u-me",
      min_score: "80",
      tag_id: ["t-1"],
      status: "disqualified",
      include_archived: true,
    });
  });
});

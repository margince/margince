import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  distinctFields,
  groupByField,
  provenanceOfEntry,
} from "./history.logic";

type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

const e = (
  field: string,
  at: string,
  actor: FieldHistoryEntry["actor_type"] = "human",
) =>
  ({
    id: at,
    entity_type: "deal",
    entity_id: "d1",
    field,
    old_value: null,
    new_value: "x",
    changed_at: at,
    actor_type: actor,
    actor_id: "u1",
  }) as const;

describe("groupByField", () => {
  it("groups entries by field, newest-first within a group, first-seen field order", () => {
    const groups = groupByField([
      e("name", "2026-01-01"),
      e("amount", "2026-01-02"),
      e("name", "2026-03-01"),
    ]);
    expect(groups.map((g) => g.field)).toEqual(["name", "amount"]);
    expect(groups[0].changes.map((c) => c.changed_at)).toEqual([
      "2026-03-01",
      "2026-01-01",
    ]);
  });
  it("returns [] for no entries", () => {
    expect(groupByField([])).toEqual([]);
  });
});

describe("distinctFields", () => {
  it("lists fields in first-seen order without dupes", () => {
    expect(
      distinctFields([e("name", "1"), e("amount", "2"), e("name", "3")]),
    ).toEqual(["name", "amount"]);
  });
});

describe("provenanceOfEntry", () => {
  const passport = "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69";

  it("names the colleague an agent worked for and the tool, from the row itself", () => {
    expect(
      provenanceOfEntry({
        actor_type: "agent",
        actor_id: passport,
        on_behalf_of_name: "Anna Weber",
        agent_client: "Claude",
      }),
    ).toEqual({ kind: "agent", agent: "Claude", forName: "Anna Weber" });
  });

  it("keeps the unnamed tag for an agent with no colleague behind it", () => {
    expect(
      provenanceOfEntry({ actor_type: "agent", actor_id: passport }),
    ).toEqual({ kind: "agent" });
  });

  it("adds no colleague to a connector's change", () => {
    expect(
      provenanceOfEntry({
        actor_type: "connector",
        actor_id: "connector:gmail",
        on_behalf_of_name: "Anna Weber",
      }),
    ).toEqual({ kind: "connector", connector: "gmail" });
  });
});

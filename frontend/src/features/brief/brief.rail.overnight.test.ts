import { describe, expect, it } from "vitest";
import { netPhaseMoves } from "./brief.rail.overnight";

const move = (
  project: string,
  from: string | null,
  to: string,
  at: string,
) => ({
  project_id: project,
  name: project,
  from_phase: from,
  to_phase: to,
  occurred_at: at,
});

describe("the overnight projects list", () => {
  it("shows one line per project, from where it stood to where it stands", () => {
    const lines = netPhaseMoves([
      move("connox", "pursuing", "delivering", "2026-09-26T19:11:08.06Z"),
      move("connox", null, "initiative", "2026-09-26T19:11:08.05Z"),
      move("connox", "delivering", "closed", "2026-09-26T19:11:08.07Z"),
      move("connox", "initiative", "pursuing", "2026-09-26T19:11:08.055Z"),
      move("odasie", "initiative", "pursuing", "2026-09-26T19:11:09Z"),
    ]);
    expect(lines).toHaveLength(2);
    const connox = lines.find((line) => line.project_id === "connox");
    expect(connox?.from_phase).toBe("initiative");
    expect(connox?.to_phase).toBe("closed");
  });

  it("drops a project that only came into being, or ended where it began", () => {
    expect(
      netPhaseMoves([
        move("born", null, "initiative", "2026-09-26T08:00:00Z"),
        move("back", "pursuing", "delivering", "2026-09-26T08:00:00Z"),
        move("back", "delivering", "pursuing", "2026-09-26T09:00:00Z"),
      ]),
    ).toEqual([]);
  });
});

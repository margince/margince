import { describe, expect, it } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import type { Project360 } from "./projectsections";
import { commitmentsPeek, timePeek } from "./projectstanding.peeks";

const fill = (text: string, params?: Record<string, string>) =>
  Object.entries(params ?? {}).reduce(
    (out, [name, value]) => out.replace(`{${name}}`, value),
    text,
  );
const words = {
  t: (key: keyof typeof en, params?: Record<string, string>) =>
    fill(en[key], params),
  plural: (base: string, count: number, params?: Record<string, string>) =>
    fill(
      en[`${base}_${count === 1 ? "one" : "other"}` as keyof typeof en],
      params,
    ),
  locale: "en",
  zone: UTC_ZONE,
} as never;

const view = (over: object): Project360 =>
  ({
    sections_omitted: [],
    as_of: "2026-10-22T09:00:00Z",
    project: {
      phase: "delivering",
      started_at: "2026-05-01",
      target_end_date: "2026-11-30",
    },
    ...over,
  }) as unknown as Project360; // contract:cast only the fields this peek reads are filled

describe("commitmentsPeek", () => {
  it("says how many of the open tasks the page shows when the rollup counts more", () => {
    const peek = commitmentsPeek(
      view({
        commitments: {
          data: [
            {
              activity_id: "a",
              subject: "Kickoff agenda",
              due_at: "2026-10-10T00:00:00Z",
              assignee_name: "Demo Admin",
              overdue: true,
              assignee_id: null,
            },
          ],
          page: { has_more: true },
        },
        rollups: { open_commitments: 3 },
      }),
      words,
      () => undefined,
    );

    expect(peek?.items[0]?.sub).toBe("Demo Admin · 10 Oct, overdue");
    expect(peek?.foot).toBe("Showing 1 of 3.");
  });
});

describe("timePeek", () => {
  it("shows the start, today and the target end that the day count is made of", () => {
    const peek = timePeek(view({}), words);

    expect(peek?.items.map((item) => item.sub)).toEqual([
      "1 May",
      "Day 175 of 214",
      "30 Nov",
    ]);
  });

  it("offers nothing for a project with no dates", () => {
    expect(
      timePeek(view({ project: { phase: "delivering" } }), words),
    ).toBeUndefined();
  });
});

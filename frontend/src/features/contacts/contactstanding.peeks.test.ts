import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import { contactPeeks } from "./contactstanding.peeks";

type Contact360 = components["schemas"]["Contact360"];

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

const mail = (
  id: string,
  at: string,
  direction: "inbound" | "outbound",
  precision: "instant" | "day" = "instant",
) => ({
  id,
  kind: "email",
  occurred_at: at,
  occurred_precision: precision,
  direction,
  subject: `Subject ${id}`,
  email_summary: { direction, move: "none" },
});

const view = (over: object): Contact360 =>
  ({
    sections_omitted: [],
    as_of: "2026-10-22T09:00:00Z",
    ...over,
  }) as unknown as Contact360; // contract:cast only the fields this peek reads are filled

describe("contactPeeks", () => {
  it("shows as many messages as the 90 day counts say, newest first, and none older", () => {
    const { exchange } = contactPeeks(
      view({
        strength: { inbound_90d: 1, outbound_90d: 1 },
        activities: {
          data: [
            mail("new", "2026-10-19T00:00:00Z", "inbound"),
            mail("mid", "2026-10-15T00:00:00Z", "outbound"),
            mail("old", "2026-03-01T00:00:00Z", "outbound"),
          ],
        },
      }),
      words,
      () => undefined,
    );

    expect(exchange?.items.map((item) => item.key)).toEqual(["new", "mid"]);
    expect(exchange?.foot).toBeUndefined();
  });

  // A `day` entry is stored at noon on the record's clock, which can be later
  // than the read. Its day has begun, so it is a message already exchanged.
  it("counts a date-only message from today, though its noon is still ahead", () => {
    const { exchange } = contactPeeks(
      view({
        strength: { inbound_90d: 1, outbound_90d: 0 },
        activities: {
          data: [mail("today", "2026-10-22T12:00:00Z", "inbound", "day")],
        },
      }),
      words,
      () => undefined,
    );

    expect(exchange?.items.map((item) => item.key)).toEqual(["today"]);
  });

  it("lists only the colleagues the Reach reading counts as in touch", () => {
    const { reach } = contactPeeks(
      view({
        network: {
          colleagues: [
            {
              user_id: "u1",
              display_name: "Demo Admin",
              interactions_90d: 9,
              inbound_90d: 4,
              outbound_90d: 5,
              two_way: true,
              last_at: "2026-10-20T00:00:00Z",
            },
            {
              user_id: "u2",
              display_name: "Inbound-only Colleague",
              interactions_90d: 1,
              inbound_90d: 1,
              outbound_90d: 0,
              two_way: false,
            },
          ],
        },
      }),
      words,
      () => undefined,
    );

    expect(reach?.items.map((item) => item.title)).toEqual(["Demo Admin"]);
  });

  it("lists only open commitments of ours, and marks a slipped one", () => {
    const { promises } = contactPeeks(
      view({
        claims: [
          {
            id: "1",
            kind: "commitment_ours",
            status: "open",
            body: "Send the model",
            due_at: "2026-10-01T00:00:00Z",
          },
          {
            id: "2",
            kind: "commitment_ours",
            status: "done",
            body: "Done thing",
          },
          {
            id: "3",
            kind: "commitment_theirs",
            status: "open",
            body: "Theirs",
          },
        ],
      }),
      words,
      () => undefined,
    );

    expect(promises?.items).toHaveLength(1);
    expect(promises?.items[0]?.sub).toBe("1 Oct, overdue");
  });
});

import { describe, expect, it } from "vitest";
import { subscribableEventTypeValues } from "../api/public-events";
import { en } from "../i18n/en";
import {
  eventGroupLabelKey,
  eventLabelKey,
  eventWhenKey,
  WEBHOOK_EVENT_GROUPS,
} from "./webhookevents";

describe("the webhook event groups", () => {
  it("place every subscribable event type in exactly one group", () => {
    const grouped = WEBHOOK_EVENT_GROUPS.flatMap((group) => group.types);
    expect([...grouped].sort()).toEqual(
      [...subscribableEventTypeValues].sort(),
    );
  });

  it("name every group and event in the catalog", () => {
    const missing = WEBHOOK_EVENT_GROUPS.flatMap((group) => [
      eventGroupLabelKey(group.key),
      ...group.types.flatMap((type) => [
        eventLabelKey(type),
        eventWhenKey(type),
      ]),
    ]).filter((key) => !(key in en));
    expect(missing).toEqual([]);
  });
});

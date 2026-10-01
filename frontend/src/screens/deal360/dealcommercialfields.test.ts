import { expect, it } from "vitest";
import type { MessageKey } from "../../i18n/en";
import { en } from "../../i18n/en";
import {
  dealCommercialFields,
  MOTION_OPTIONS,
  PRIORITY_OPTIONS,
} from "./dealcommercialfields";

const t = (key: MessageKey): string => en[key];

it("shows the brief's guidance as catalog text, not its key", () => {
  const brief = dealCommercialFields(t, {
    motionOptions: MOTION_OPTIONS,
    priorityOptions: PRIORITY_OPTIONS,
    sources: [],
  }).find((field) => field.key === "description");

  expect(brief?.hint).toBe("Customer need, scope and intended outcome.");
});

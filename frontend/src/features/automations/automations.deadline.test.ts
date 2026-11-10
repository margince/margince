import { describe, expect, it } from "vitest";
import { paramFields, paramsFromValues } from "./automations.params";

const schema = {
  type: "object",
  properties: {
    due_in_days: { type: "integer", default: 1 },
    timezone: { type: "string", format: "timezone" },
  },
};

describe("calendar-day automation deadlines", () => {
  it("uses the timezone picker declared by the server", () => {
    expect(
      paramFields(schema).find((field) => field.key === "timezone")?.kind,
    ).toBe("timezone");
  });
  it("keeps an existing rule's deadline when no timezone was selected", () => {
    expect(
      paramsFromValues(paramFields(schema), { due_in_days: "3", timezone: "" }),
    ).toEqual({ due_in_days: 3 });
  });
  it("saves the chosen timezone beside the calendar offset", () => {
    expect(
      paramsFromValues(paramFields(schema), {
        due_in_days: "1",
        timezone: "Europe/Berlin",
      }),
    ).toEqual({ due_in_days: 1, timezone: "Europe/Berlin" });
  });
});

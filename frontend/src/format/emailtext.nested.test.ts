import { describe, expect, it } from "vitest";
import cases from "../../../backend/internal/shared/kernel/mailquote/testdata/nested-forwards.json";
import { splitEmailBody } from "./emailtext";

describe("nested forwards shared with the server", () => {
  it("contains nested clients and ordinary reply history", () => {
    expect(cases.length).toBeGreaterThanOrEqual(4);
  });
  it.each(cases)("$name", ({ subject, body, main, trimmed }) => {
    expect(splitEmailBody(body, subject)).toMatchObject({ main, trimmed });
  });
});

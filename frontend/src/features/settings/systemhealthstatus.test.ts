// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { healthReport, overdueFinding } from "./systemhealth.fixtures";
import { healthTone } from "./systemhealthstatus";

it("uses the same failure severity for findings and source readings", () => {
  expect(healthTone(overdueFinding)).toBe("danger");
  expect(
    healthTone({
      ...overdueFinding,
      code: "read_failed",
      condition: "unknown",
    }),
  ).toBe("danger");
  expect(
    healthTone({
      ...overdueFinding,
      code: "queue_unverified",
      condition: "unverified",
    }),
  ).toBe("danger");
  expect(
    healthTone({
      ...overdueFinding,
      code: "capture_uncertain",
      condition: "waiting_human",
    }),
  ).toBe("warning");
});

it("requires a verified available clear check for green", () => {
  const source = healthReport.sources.find((source) => source.id === "capture");
  if (!source) throw new Error("Capture fixture missing");
  expect(healthTone(source)).toBe("success");
  expect(healthTone({ ...source, availability: "not_monitored" })).toBe(
    "default",
  );
  expect(healthTone({ ...source, condition: "processing" })).toBe("default");
  expect(healthTone({ ...source, availability: "failed" })).toBe("danger");
});

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { STEP_MS } from "./voice-distill";

// The panel's motion is spelled on both sides of one wire: the ticker's step
// is a number in TypeScript, the glide a duration in CSS, and only their
// RELATIONSHIP matters — a glide that outlasts the step leaves a line still
// arriving when the next one is due, which is the stutter the animation was
// rebuilt to remove. Nothing else compares them, so this does.

const sheet = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "conversation.css"),
  "utf8",
);

function durationOf(property: string): number {
  const match = new RegExp(`${property}:\\s*(\\d+)ms`).exec(sheet);
  if (match === null) {
    throw new Error(`${property} is not declared in conversation.css`);
  }
  return Number(match[1]);
}

describe("the distilling panel's pacing", () => {
  it("finishes a line's arrival well inside the step that brings the next", () => {
    const glide = durationOf("--distill-glide");
    expect(glide).toBeGreaterThan(0);
    // Half the step, not merely under it: at the boundary the panel is always
    // mid-animation, which reads as constant motion rather than as lines
    // arriving.
    expect(glide).toBeLessThanOrEqual(STEP_MS / 2);
  });
});

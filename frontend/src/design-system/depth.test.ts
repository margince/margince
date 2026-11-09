import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

describe("depth", () => {
  // The other way this gate fails short. Reading a custom property nothing
  // declares is not a CSS error: the declaration is dropped and the shadow
  // simply stops being drawn, in silence, on every surface at once. Every rule
  // above reads the name, so the name has to exist in every theme arm — not
  // only in the one whose screenshot somebody happened to look at.
  it.each(["--shadow-rest", "--shadow-well", "--shadow-pop"])(
    "declares %s in every theme arm",
    (token) => {
      const tokens = readFileSync(
        join(frontendRoot, "src", "design-system", "tokens.css"),
        "utf8",
      );
      expect(
        [...tokens.matchAll(new RegExp(`${token}:\\s*[^;]+;`, "g"))].length,
        `${token} belongs in the light block, the dark toggle and the ` +
          "platform-preference arm; a theme missing it loses that depth " +
          "everywhere at once, and says nothing about it",
      ).toBe(3);
    },
  );
});

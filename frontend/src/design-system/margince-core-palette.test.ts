// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";
import { runCoreLoop, type Wanted } from "./margince-core-engine";
import type { CoreFrame, CoreRenderer } from "./margince-core-gl";
import { BEHAVIOUR, rowFor } from "./margince-core-motion";
import {
  CORE_FRAG,
  CORE_POINTS_FRAG,
  CORE_POINTS_VERT,
} from "./margince-core-shader";

// The regression this guards: a shader that holds its palette as GLSL
// literals cannot be repainted by tokens.css at all, and the shader alone
// still compiles and still draws, so nothing in a render test would ever
// notice. Each case below fails for a different reason a repaint could stop
// reaching the ball: the uniforms not declared, a literal still standing
// where a read should be, a token missing from tokens.css, or the engine
// never handing the read palette down to a frame.

const here = dirname(fileURLToPath(import.meta.url));

describe("the Core's palette, as uniforms", () => {
  it("declares uWork and uBody rather than holding the palette as literals", () => {
    expect(CORE_FRAG).toMatch(/uniform\s+vec3\s+uWork\s*\[\s*5\s*\]/);
    expect(CORE_FRAG).toMatch(/uniform\s+vec3\s+uBody\s*\[\s*2\s*\]/);
    expect(CORE_POINTS_VERT).toMatch(/uniform\s+vec3\s+uWork\s*\[\s*5\s*\]/);
  });

  it("colours the ribbons from uWork rather than from a literal colour", () => {
    const tone = /vec3 tone\(float x\)\{[\s\S]*?\n\}/.exec(
      CORE_POINTS_VERT,
    )?.[0];
    if (!tone) {
      throw new Error("the points shader has no tone() to check");
    }
    expect(tone).not.toMatch(/vec3\(0\./);
    expect(tone).toContain("uWork");
  });

  it("mixes the ball's body from uBody rather than from a literal colour", () => {
    const ballLine = /vec3 ball = mix\([^;]+\);/.exec(CORE_FRAG)?.[0];
    if (!ballLine) {
      throw new Error("the shader has no `ball = mix(...)` line to check");
    }
    expect(ballLine).not.toMatch(/vec3\(0\./);
    expect(ballLine).toContain("uBody");
  });
});

describe("the tokens the palette reads", () => {
  const tokensCss = readFileSync(join(here, "tokens.css"), "utf8");
  const tokens = [
    "--ai",
    "--orbGlow",
    "--orbMid",
    "--orbAmber",
    "--orbBright",
    "--orbDeep",
    "--orbInk",
  ];

  it.each(tokens)(
    "%s exists in tokens.css with a 6-digit hex value",
    (name) => {
      const declared = new RegExp(`${name}:\\s*#[0-9a-f]{6}\\b`, "i");
      expect(tokensCss).toMatch(declared);
    },
  );
});

describe("the palette reaching the renderer", () => {
  // The engine's own factory seam: a stub stands in for the GPU so this test
  // can ask what a frame carried without a real WebGL2 context.
  function recorder(): CoreRenderer & { frames: CoreFrame[] } {
    const frames: CoreFrame[] = [];
    return {
      frames,
      resize: () => {},
      draw: (frame) => {
        frames.push(frame);
      },
      dispose: () => {},
    };
  }

  function hexToRgb(hex: string): readonly [number, number, number] {
    const value = Number.parseInt(hex.replace("#", ""), 16);
    return [
      ((value >> 16) & 255) / 255,
      ((value >> 8) & 255) / 255,
      (value & 255) / 255,
    ];
  }

  function setToken(name: string, hex: string) {
    document.documentElement.style.setProperty(name, hex);
  }

  it("hands the renderer five work stops and two body stops, matching the tokens", () => {
    setToken("--ai", "#112233");
    setToken("--orbGlow", "#445566");
    setToken("--orbMid", "#667788");
    setToken("--orbAmber", "#778899");
    setToken("--orbBright", "#aabbcc");
    setToken("--orbDeep", "#0a0b0c");
    setToken("--orbInk", "#0d0e0f");

    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const canvas = document.createElement("canvas");
    const renderer = recorder();
    const wanted: { current: Wanted } = {
      current: { behaviour: rowFor("idle"), paper: 0 },
    };
    const loop = runCoreLoop(canvas, wanted, () => renderer);
    if (!loop) {
      throw new Error("the loop refused a renderer that was handed to it");
    }
    loop.stop();

    const frame = renderer.frames[0];
    if (!frame) {
      throw new Error("mounting the loop drew no frame at all");
    }

    expect(frame.work).toHaveLength(5);
    expect(frame.body).toHaveLength(2);
    // The ribbon stops A to D, then the highlight tint.
    expect(frame.work[0]).toEqual(hexToRgb("#112233"));
    expect(frame.work[1]).toEqual(hexToRgb("#445566"));
    expect(frame.work[2]).toEqual(hexToRgb("#667788"));
    expect(frame.work[3]).toEqual(hexToRgb("#778899"));
    expect(frame.work[4]).toEqual(hexToRgb("#aabbcc"));
    expect(frame.body[0]).toEqual(hexToRgb("#0a0b0c"));
    expect(frame.body[1]).toEqual(hexToRgb("#0d0e0f"));

    vi.restoreAllMocks();
  });
});

// GLSL does not run in vitest, so the cases below pin the shader's source text.
describe("the gold cluster", () => {
  // uWork[3] is the stop the palette fills from --orbAmber (pinned above).
  it("colours the gold ribbon from the amber stop, not from a literal", () => {
    expect(CORE_POINTS_VERT).toMatch(
      /col = mix\(col, uWork\[3\], [^;]*gold\);/,
    );
    expect(CORE_POINTS_VERT).toContain("vGold = gold;");
  });

  it("carries only the gold ribbon's light in the buffer's alpha", () => {
    expect(CORE_POINTS_FRAG).toContain(
      "outColor = vec4(vColor * a, a * vGold * dot(vColor, vec3(0.2126, 0.7152, 0.0722)));",
    );
  });

  it("keeps the gold's hue where its share of the light is high", () => {
    expect(CORE_FRAG).toContain(
      "float goldShare = clamp(mid4.a / max(midL, 1e-4), 0.0, 1.0);",
    );
    expect(CORE_FRAG).toContain(
      "body = mix(body, goldLit, smoothstep(0.1, 0.55, goldShare) * 0.95);",
    );
  });

  it("mixes the state's tint into the gold's colour", () => {
    expect(CORE_FRAG).toContain(
      "vec3 goldCol = mix(uWork[3], uTintCol, uTint);",
    );
  });

  it.each(["warning", "error"] as const)(
    "gives %s a full tint, so its gold takes the tint colour",
    (state) => {
      expect(BEHAVIOUR[state].tint).toBe(1);
    },
  );
});

describe("the lens", () => {
  it("bends nothing in the middle and ramps in over the outer quarter", () => {
    const bend = /float bend = ([^;]+);/.exec(CORE_FRAG)?.[1];
    expect(bend).toBe("pow(smoothstep(0.75, 1.0, pd), 1.5)");
    expect(CORE_FRAG).toContain("vec2 q = p - n * 0.70 * bend;");
    expect(CORE_FRAG).toContain("float split = 0.030 * bend;");
  });
});

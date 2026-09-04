// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { WAVES_FRAG } from "./ambient-waves-shader";
import {
  bindFullscreenProgram,
  createFullscreenProgram,
} from "./webgl-program";

/**
 * The WebGL2 half of the welcome ground: context, uniforms, one draw call.
 *
 * Everything here is mechanics. What the ground looks like is the shader's, and
 * when it moves is the component's.
 */

/** What one frame of the ground is drawn from. */
export type WavesFrame = Readonly<{
  /** Seconds since the ground started, accumulated by the caller. */
  time: number;
  /** 0..1 arrival, so the ground rises out of the paper it sits on. */
  fade: number;
  /** The surface underneath, read off the tokens. */
  paper: readonly [number, number, number];
  /** Three band hues, read off the tokens, palest first. */
  hues: readonly (readonly [number, number, number])[];
}>;

export type WavesRenderer = Readonly<{
  /** Sizes the drawing buffer. Cheap when nothing changed. */
  resize: (cssWidth: number, cssHeight: number) => void;
  draw: (frame: WavesFrame) => void;
  dispose: () => void;
}>;

const UNIFORMS = ["iResolution", "uTime", "uFade", "uPaper", "uHue"] as const;

type UniformName = (typeof UNIFORMS)[number];

/** How many band hues the shader's `uHue` array holds. */
const BANDS = 3;

/**
 * Device pixels per CSS pixel for the ground.
 *
 * ONE, not the display's. Capping at CSS resolution is worth roughly four times
 * the fill on a retina panel, and this shader samples simplex noise five times
 * per pixel over a whole viewport: a real cost to pay behind a sign-in form.
 * Going FURTHER down was tried and reverted: the ribbons are shaped by a warp
 * field, and drawing that small and scaling it up softens the folds back into
 * the blobs the warp exists to get rid of.
 */
const RENDER_SCALE = 1;

/** Flattens the band palette into the flat float list `uniform3fv` wants. */
function flatten(
  stops: readonly (readonly [number, number, number])[],
): Float32Array {
  const flat = new Float32Array(BANDS * 3);
  for (let i = 0; i < BANDS; i++) {
    // A stop the caller did not supply reads mid-grey rather than black, so a
    // short palette looks wrong rather than looking like a hole.
    const stop = stops[i] ?? [0.5, 0.5, 0.5];
    flat[i * 3] = stop[0];
    flat[i * 3 + 1] = stop[1];
    flat[i * 3 + 2] = stop[2];
  }
  return flat;
}

function locate(
  gl: WebGL2RenderingContext,
  program: WebGLProgram,
): Record<UniformName, WebGLUniformLocation | null> {
  const found = {} as Record<UniformName, WebGLUniformLocation | null>;
  for (const name of UNIFORMS) {
    found[name] = gl.getUniformLocation(program, name);
  }
  return found;
}

/**
 * Builds the ground's renderer for one canvas, or returns null with the reason
 * logged.
 *
 * Opaque (`alpha: false`): the shader already draws the page's own paper as its
 * base, so there is nothing for the compositor to blend and asking it to blend
 * anyway costs a full-screen pass on every frame.
 */
export function createWavesRenderer(
  canvas: HTMLCanvasElement,
): WavesRenderer | null {
  const gl = canvas.getContext("webgl2", {
    alpha: false,
    antialias: false,
    depth: false,
    stencil: false,
    powerPreference: "low-power",
  });
  if (!gl) {
    // Not an error: jsdom and every pre-WebGL2 browser land here, and the
    // surface has a CSS ground underneath for exactly this.
    return null;
  }
  const program = createFullscreenProgram(gl, WAVES_FRAG, "Welcome ground");
  if (!program) {
    return null;
  }
  const vao = bindFullscreenProgram(gl, program);
  const at = locate(gl, program);

  return {
    resize(cssWidth, cssHeight) {
      const w = Math.max(1, Math.round(cssWidth * RENDER_SCALE));
      const h = Math.max(1, Math.round(cssHeight * RENDER_SCALE));
      if (canvas.width === w && canvas.height === h) {
        return;
      }
      canvas.width = w;
      canvas.height = h;
      gl.viewport(0, 0, w, h);
    },
    draw(frame) {
      gl.uniform2f(at.iResolution, canvas.width, canvas.height);
      gl.uniform1f(at.uTime, frame.time);
      gl.uniform1f(at.uFade, frame.fade);
      gl.uniform3f(at.uPaper, frame.paper[0], frame.paper[1], frame.paper[2]);
      gl.uniform3fv(at.uHue, flatten(frame.hues));
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    },
    dispose() {
      // The GPU objects go; the CONTEXT stays. Forcing it lost frees it sooner,
      // but the loss is permanent for that canvas element, and React re-runs an
      // effect on every dependency change and twice at mount under StrictMode,
      // so the ground would come back as bare paper and never recover.
      gl.deleteVertexArray(vao);
      gl.deleteProgram(program);
    },
  };
}

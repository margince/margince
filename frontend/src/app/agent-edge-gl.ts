// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EDGE_FRAG, EDGE_VERT } from "./agent-edge-shader";
import type { AgentEdgeRegister } from "./agent-edge-signal";

/**
 * The edge's renderer: one program, one quad, one draw call per frame.
 *
 * Deliberately its own file rather than a second caller of the Core's renderer
 * (`design-system/margince-core-gl.ts`). That one owns a sphere: its uniforms are
 * dials on a lit ball and its resize reasons about a square buffer. Sharing it
 * would mean a union of two unrelated uniform sets and a size contract that suits
 * neither, which is how a seam becomes a knot.
 *
 * Never throws. Every failure returns null and the caller wears a static rim
 * instead: a locked-down browser, a refused compile and a GPU with no WebGL2 are
 * all the same answer to a decoration.
 */

export type EdgeFrame = Readonly<{
  /** Seconds since the edge lit. */
  time: number;
  /** 0 while dark, 1 while fully lit: the fade lives here, not in CSS opacity. */
  level: number;
  /**
   * The rim's resting thickness this frame, in CSS pixels: a register's own, or
   * a point between two while the loop eases from one to the other.
   */
  thick: number;
  /**
   * How much the rim breathes: how far the crests swell it and how far the
   * halo reaches with them. 1 is the amplitude the shader was tuned at.
   */
  wave: number;
  /**
   * How much of the travelling head to draw: the register's share normally, 0
   * under reduced motion.
   *
   * Per frame rather than fixed at construction because it is the loop that knows
   * about motion preference and which register is wanted, and because a test can
   * then read what the loop asked for without a GPU.
   */
  beam: number;
}>;

export type EdgeRenderer = Readonly<{
  /** Sizes the drawing buffer. Cheap when nothing changed. */
  resize: (cssWidth: number, cssHeight: number, dpr: number) => void;
  draw: (frame: EdgeFrame) => void;
  dispose: () => void;
}>;

/** What the rim draws at in one register: the three dials the loop turns. */
export type EdgeDress = Readonly<{
  /** Resting thickness in CSS pixels, before the wave swells it. */
  thick: number;
  /** How much of the travelling head to draw, 0..1. */
  beam: number;
  /** How much the rim breathes, 1 being the amplitude the shader was tuned at. */
  wave: number;
}>;

/**
 * The two registers, side by side so the difference between them can be read as
 * a difference: the numbers only mean anything relative to each other.
 *
 * The thickness is a floor rather than the whole width: the wave adds better
 * than half of it again at a crest, so the visible rim breathes either side of
 * it. Anything under about one and a half is mostly its own anti-aliasing, since
 * the shader's `smoothstep` spends most of a pixel on each side of the boundary,
 * which is what puts the import's thickness where it is: as thin as a rim can be
 * and still be a rim. The agent's is a little over what one register used to
 * draw at, and a little livelier, because two registers only read as two if the
 * louder one is unmistakably the louder one.
 *
 * The import keeps a faint head and half the swell rather than none of either.
 * A rim that does not move at all is the fallback a host without WebGL2 wears,
 * and it says the work is happening without being able to say it is happening
 * NOW; the import is a run that can stall, so it keeps the part of the picture
 * that only a live loop can draw.
 */
export const EDGE_REGISTERS: Readonly<Record<AgentEdgeRegister, EdgeDress>> = {
  agent: { thick: 3, beam: 1, wave: 1.15 },
  capture: { thick: 1.6, beam: 0.35, wave: 0.45 },
};

/**
 * Above 1.5 this costs fill for nothing a reader can see. The rim is a soft
 * gradient, not text, and the falloff is computed in CSS pixels either way, so a
 * 3x buffer draws the same edge three times over.
 */
const MAX_DPR = 1.5;

/** The five stops, read off the document so the tokens stay the one source. */
export type EdgeHues = readonly [
  readonly [number, number, number],
  readonly [number, number, number],
  readonly [number, number, number],
  readonly [number, number, number],
  readonly [number, number, number],
];

function compile(
  gl: WebGL2RenderingContext,
  kind: number,
  source: string,
): WebGLShader | null {
  const shader = gl.createShader(kind);
  if (!shader) {
    return null;
  }
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    // Reported rather than swallowed: a shader that fails to compile is a bug in
    // this file, and the message is the only thing that says which line.
    console.error("agent edge: shader refused to compile", {
      log: gl.getShaderInfoLog(shader),
    });
    gl.deleteShader(shader);
    return null;
  }
  return shader;
}

export function createEdgeRenderer(
  canvas: HTMLCanvasElement,
  hues: EdgeHues,
): EdgeRenderer | null {
  const gl = canvas.getContext("webgl2", {
    alpha: true,
    premultipliedAlpha: true,
    antialias: false,
    depth: false,
    stencil: false,
    // The edge is redrawn every frame it is visible, so keeping the last one
    // costs memory bandwidth for a buffer nobody reads.
    preserveDrawingBuffer: false,
    powerPreference: "low-power",
  });
  if (!gl) {
    return null;
  }
  const vert = compile(gl, gl.VERTEX_SHADER, EDGE_VERT);
  const frag = compile(gl, gl.FRAGMENT_SHADER, EDGE_FRAG);
  if (!vert || !frag) {
    return null;
  }
  const program = gl.createProgram();
  gl.attachShader(program, vert);
  gl.attachShader(program, frag);
  gl.linkProgram(program);
  // The shaders are the program's now; keeping the handles would leak them.
  gl.deleteShader(vert);
  gl.deleteShader(frag);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    console.error("agent edge: program refused to link", {
      log: gl.getProgramInfoLog(program),
    });
    gl.deleteProgram(program);
    return null;
  }

  const vao = gl.createVertexArray();
  gl.bindVertexArray(vao);
  const buffer = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
  // Two triangles covering clip space. The fragment shader does all the work, so
  // the geometry is the cheapest thing that can cover the viewport.
  gl.bufferData(
    gl.ARRAY_BUFFER,
    new Float32Array([-1, -1, 3, -1, -1, 3]),
    gl.STATIC_DRAW,
  );
  const aPos = gl.getAttribLocation(program, "aPos");
  gl.enableVertexAttribArray(aPos);
  gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

  // biome-ignore lint/correctness/useHookAtTopLevel: gl.useProgram is a WebGL call, not a React hook; the rule matches on the use* name.
  gl.useProgram(program);
  const uRes = gl.getUniformLocation(program, "uRes");
  const uDpr = gl.getUniformLocation(program, "uDpr");
  const uTime = gl.getUniformLocation(program, "uTime");
  const uLevel = gl.getUniformLocation(program, "uLevel");
  const uThick = gl.getUniformLocation(program, "uThick");
  const uWave = gl.getUniformLocation(program, "uWave");
  const uBeam = gl.getUniformLocation(program, "uBeam");
  // Through a Float32Array rather than casting the tuples: `uniform3fv` wants a
  // mutable float list, and an assertion to get one would be a claim about a
  // readonly value that is simply untrue.
  for (const [index, name] of [
    "uHueA",
    "uHueB",
    "uHueC",
    "uHueD",
    "uHueE",
  ].entries()) {
    gl.uniform3fv(
      gl.getUniformLocation(program, name),
      new Float32Array(hues[index] ?? [0.5, 0.5, 0.5]),
    );
  }

  // Premultiplied source over destination, matching the context's own
  // premultiplied alpha: the shader hands out colours already multiplied.
  gl.enable(gl.BLEND);
  gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);

  let width = 0;
  let height = 0;

  return {
    resize(cssWidth, cssHeight, dpr) {
      const ratio = Math.min(dpr || 1, MAX_DPR);
      const next = [
        Math.max(1, Math.round(cssWidth * ratio)),
        Math.max(1, Math.round(cssHeight * ratio)),
      ] as const;
      if (next[0] === width && next[1] === height) {
        return;
      }
      [width, height] = next;
      canvas.width = width;
      canvas.height = height;
      gl.viewport(0, 0, width, height);
      gl.uniform2f(uRes, width, height);
      gl.uniform1f(uDpr, ratio);
    },
    draw({ time, level, thick, wave, beam }) {
      gl.uniform1f(uTime, time);
      gl.uniform1f(uLevel, level);
      gl.uniform1f(uThick, thick);
      gl.uniform1f(uWave, wave);
      gl.uniform1f(uBeam, beam);
      // No clear: every fragment is written, and the blend is over transparent
      // black, so clearing would be a full-screen write for nothing.
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    },
    dispose() {
      // The GPU objects go; the CONTEXT stays. A canvas whose context was
      // deliberately killed answers `getContext` with null forever after, which
      // in StrictMode's double mount means the second mount gets nothing.
      gl.deleteBuffer(buffer);
      gl.deleteVertexArray(vao);
      gl.deleteProgram(program);
    },
  };
}

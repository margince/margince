// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CORE_FRAG, CORE_VERT } from "./margince-core-shader";

/**
 * The WebGL2 half of the Core: context, program, uniforms, one draw call.
 *
 * Everything here is mechanics. Nothing decides how the Core LOOKS (that is the
 * shader) or WHEN it moves (that is the engine); this file exists so neither of
 * those has to carry the shape of the API.
 *
 * It never throws. A host without WebGL2, a driver that refuses to compile, a
 * context lost to a GPU reset: each is a real state on real machines, and the
 * answer to all three is the same one the Core needs anyway, which is for the
 * component to fall back to its static dress. So the constructor returns null
 * and says why in the console, rather than taking a page down over an
 * ornament.
 */

/** The dials the shader is driven by, in the numbers it wants them in. */
export type CoreFrame = Readonly<{
  level: number;
  phase: number;
  pulse: number;
  ingest: number;
  tint: number;
  tintCol: readonly [number, number, number];
  mouse: readonly [number, number];
  /** 0 = the ball glows on a dark surface, 1 = opaque and dark on paper. */
  paper: number;
  /** The ribbon palette, read off the tokens: five stops, in the order the
   *  shader's uWork expects. Carried per frame so a theme change reaches the
   *  ball without rebuilding the GL program. */
  work: readonly (readonly [number, number, number])[];
  /** The ball's own base gradient, read off the tokens: deep then ink. */
  body: readonly (readonly [number, number, number])[];
}>;

export type CoreRenderer = Readonly<{
  /** Sizes the drawing buffer to the element. Cheap when nothing changed. */
  resize: (cssSize: number, dpr: number) => void;
  draw: (frame: CoreFrame) => void;
  dispose: () => void;
}>;

/** Above 2 this shader costs four times the fill for no visible gain. */
export const MAX_DPR = 2;

function compile(
  gl: WebGL2RenderingContext,
  type: number,
  source: string,
): WebGLShader | null {
  const shader = gl.createShader(type);
  if (!shader) {
    return null;
  }
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    return shader;
  }
  // The log is the only thing that makes a driver-specific compile failure
  // diagnosable, and it is gone the moment the shader is deleted.
  console.error("Margince Core shader failed to compile", {
    log: gl.getShaderInfoLog(shader),
  });
  gl.deleteShader(shader);
  return null;
}

function link(gl: WebGL2RenderingContext): WebGLProgram | null {
  const vert = compile(gl, gl.VERTEX_SHADER, CORE_VERT);
  const frag = compile(gl, gl.FRAGMENT_SHADER, CORE_FRAG);
  if (!vert || !frag) {
    return null;
  }
  const program = gl.createProgram();
  if (!program) {
    return null;
  }
  gl.attachShader(program, vert);
  gl.attachShader(program, frag);
  gl.linkProgram(program);
  // Attached shaders are kept alive by the program until it links; once it has,
  // the objects themselves are dead weight.
  gl.deleteShader(vert);
  gl.deleteShader(frag);
  if (gl.getProgramParameter(program, gl.LINK_STATUS)) {
    return program;
  }
  console.error("Margince Core program failed to link", {
    log: gl.getProgramInfoLog(program),
  });
  gl.deleteProgram(program);
  return null;
}

/**
 * Binds the program and the (empty) vertex array the draw call needs.
 *
 * Its own function so the lint rule that recognises `use*` as a React hook does
 * not read `gl.useProgram` sitting after an early return as a conditional hook.
 * Naming a GL method is not something this file gets to change.
 */
function bind(
  gl: WebGL2RenderingContext,
  program: WebGLProgram,
): WebGLVertexArrayObject | null {
  const vao = gl.createVertexArray();
  // biome-ignore lint/correctness/useHookAtTopLevel: gl.useProgram is a WebGL method, not a React hook; the rule matches the use* name.
  gl.useProgram(program);
  gl.bindVertexArray(vao);
  gl.disable(gl.DEPTH_TEST);
  return vao;
}

const UNIFORMS = [
  "iResolution",
  "uMouse",
  "uLevel",
  "uPhase",
  "uPulse",
  "uTint",
  "uTintCol",
  "uIngest",
  "uPaper",
  "uWork",
  "uBody",
] as const;

type UniformName = (typeof UNIFORMS)[number];

/** How many stops each palette uniform carries, matching the shader's arrays. */
const WORK_STOPS = 5;
const BODY_STOPS = 2;

/**
 * Flattens a palette into the flat float list `uniform3fv` wants.
 *
 * Through a Float32Array rather than a cast: the frame's stops are a readonly
 * tuple and `uniform3fv` wants a mutable float list, so there is no assertion
 * that would make one into the other honestly. A stop the caller did not
 * supply reads mid-grey rather than a hole, matching `readHue`'s own rule that
 * a missing colour should look wrong, not look absent.
 */
function flatten(
  stops: readonly (readonly [number, number, number])[],
  count: number,
): Float32Array {
  const flat = new Float32Array(count * 3);
  for (let i = 0; i < count; i++) {
    const [r, g, b] = stops[i] ?? [0.5, 0.5, 0.5];
    flat[i * 3] = r;
    flat[i * 3 + 1] = g;
    flat[i * 3 + 2] = b;
  }
  return flat;
}

/**
 * Reads a colour token off the document and hands back linear-ish 0..1 floats.
 *
 * The tokens are the one source for colour in this tree, and a shader cannot
 * read a custom property, so this is the seam that carries a value from
 * `tokens.css` into a uniform. It lives HERE, in the lower tier, and the lit
 * window edge in `app/` imports it: every shader in this tree needs the same
 * seam, and a second copy of it drifts the moment one of them learns about a
 * colour format the other does not.
 *
 * A token that resolves to nothing gets mid-grey rather than black, so a
 * missing hue looks wrong rather than looking like a hole. Six-digit hex only:
 * a named colour, or one written in a wider gamut notation, is not a parse
 * failure to paper over with a channel-wise guess. It means the token moved,
 * and this seam has to be told about it.
 */
export function readHue(name: string): readonly [number, number, number] {
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  const hex = /^#([0-9a-f]{6})$/i.exec(raw);
  if (!hex) {
    return [0.5, 0.5, 0.5];
  }
  const value = Number.parseInt(hex[1], 16);
  return [
    ((value >> 16) & 255) / 255,
    ((value >> 8) & 255) / 255,
    (value & 255) / 255,
  ];
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
 * Builds the renderer for one canvas, or returns null with the reason logged.
 *
 * `premultipliedAlpha` is on and the shader writes premultiplied output, which
 * is what lets the Core sit on any surface: on the dark rail it adds its own
 * light, and on paper it draws an opaque body with an antialiased edge. A
 * straight-alpha canvas fringes every one of those edges dark.
 */
export function createCoreRenderer(
  canvas: HTMLCanvasElement,
): CoreRenderer | null {
  const gl = canvas.getContext("webgl2", {
    alpha: true,
    premultipliedAlpha: true,
    antialias: false,
    depth: false,
    stencil: false,
    powerPreference: "low-power",
  });
  if (!gl) {
    // Not an error: jsdom and every pre-WebGL2 browser land here, and the
    // component has a static dress for exactly this.
    return null;
  }
  const program = link(gl);
  if (!program) {
    return null;
  }
  const vao = bind(gl, program);
  const at = locate(gl, program);

  return {
    resize(cssSize, dpr) {
      const side = Math.max(1, Math.round(cssSize * Math.min(dpr, MAX_DPR)));
      if (canvas.width === side && canvas.height === side) {
        return;
      }
      canvas.width = side;
      canvas.height = side;
      gl.viewport(0, 0, side, side);
    },
    draw(frame) {
      gl.uniform2f(at.iResolution, canvas.width, canvas.height);
      gl.uniform2f(at.uMouse, frame.mouse[0], frame.mouse[1]);
      gl.uniform1f(at.uLevel, frame.level);
      gl.uniform1f(at.uPhase, frame.phase);
      gl.uniform1f(at.uPulse, frame.pulse);
      gl.uniform1f(at.uTint, frame.tint);
      gl.uniform3f(
        at.uTintCol,
        frame.tintCol[0],
        frame.tintCol[1],
        frame.tintCol[2],
      );
      gl.uniform1f(at.uIngest, frame.ingest);
      gl.uniform1f(at.uPaper, frame.paper);
      gl.uniform3fv(at.uWork, flatten(frame.work, WORK_STOPS));
      gl.uniform3fv(at.uBody, flatten(frame.body, BODY_STOPS));
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    },
    dispose() {
      // The GPU objects go; the CONTEXT stays. Forcing it lost through
      // `WEBGL_lose_context` frees it sooner, but the loss is permanent for that
      // canvas element, and the same element is remounted constantly: React
      // re-runs an effect on every dependency change, and twice at mount under
      // StrictMode. A canvas whose context was deliberately killed answers
      // `getContext` with null forever after, so the Core would come back as its
      // static dress and never recover.
      gl.deleteVertexArray(vao);
      gl.deleteProgram(program);
    },
  };
}

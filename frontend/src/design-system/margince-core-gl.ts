// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  CORE_FRAG,
  CORE_POINTS_FRAG,
  CORE_POINTS_VERT,
  CORE_VERT,
} from "./margince-core-shader";

/**
 * The WebGL2 half of the Core: context, two programs, an offscreen buffer, two
 * draw calls.
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
  /** 0 = the calm idle figure, 1 = the thinking one. */
  form: number;
  phase: number;
  pulse: number;
  ingest: number;
  tint: number;
  tintCol: readonly [number, number, number];
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

/** The ribbons the points shader lays out, matching its LAYERS. */
const LAYERS = 4;

/**
 * How many points one ribbon gets, and how big they are, for a square buffer
 * of `side` device pixels.
 *
 * The count follows the pixel area, so a rail orb's grain is as fine as the
 * hero's. The shader divides each point's light by the overlap, so the
 * brightness holds across sizes.
 */
export function pointGrid(side: number): Readonly<{
  along: number;
  across: number;
  pointPx: number;
}> {
  return {
    along: Math.round(Math.min(240, Math.max(40, side * 0.35))),
    across: Math.round(Math.min(80, Math.max(14, side * 0.14))),
    pointPx: Math.min(8, Math.max(2, side * 0.0126)),
  };
}

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

function link(
  gl: WebGL2RenderingContext,
  vertSource: string,
  fragSource: string,
): WebGLProgram | null {
  const vert = compile(gl, gl.VERTEX_SHADER, vertSource);
  const frag = compile(gl, gl.FRAGMENT_SHADER, fragSource);
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
 * Binds a program for the draw that follows.
 *
 * Its own function so the lint rule that recognises `use*` as a React hook does
 * not read `gl.useProgram` sitting after an early return as a conditional hook.
 * Naming a GL method is not something this file gets to change.
 */
function select(gl: WebGL2RenderingContext, program: WebGLProgram): void {
  // biome-ignore lint/correctness/useHookAtTopLevel: gl.useProgram is a WebGL method, not a React hook; the rule matches the use* name.
  gl.useProgram(program);
}

const UNIFORMS = [
  "iResolution",
  "uLevel",
  "uForm",
  "uPhase",
  "uPulse",
  "uTint",
  "uTintCol",
  "uIngest",
  "uPaper",
  "uWork",
  "uBody",
  "uGrid",
  "uPointPx",
  "uParticles",
] as const;

type UniformName = (typeof UNIFORMS)[number];
type Locations = (name: UniformName) => WebGLUniformLocation | null;

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

function locate(gl: WebGL2RenderingContext, program: WebGLProgram): Locations {
  const found = new Map<UniformName, WebGLUniformLocation | null>(
    UNIFORMS.map((name) => [name, gl.getUniformLocation(program, name)]),
  );
  return (name) => found.get(name) ?? null;
}

/** Sets the dials both programs share; a location a program lacks is null and
 *  the call is a no-op. */
function setFrame(
  gl: WebGL2RenderingContext,
  at: Locations,
  frame: CoreFrame,
  side: number,
): void {
  gl.uniform2f(at("iResolution"), side, side);
  gl.uniform1f(at("uLevel"), frame.level);
  gl.uniform1f(at("uForm"), frame.form);
  gl.uniform1f(at("uPhase"), frame.phase);
  gl.uniform1f(at("uPulse"), frame.pulse);
  gl.uniform1f(at("uTint"), frame.tint);
  gl.uniform3f(
    at("uTintCol"),
    frame.tintCol[0],
    frame.tintCol[1],
    frame.tintCol[2],
  );
  gl.uniform1f(at("uIngest"), frame.ingest);
  gl.uniform1f(at("uPaper"), frame.paper);
  gl.uniform3fv(at("uWork"), flatten(frame.work, WORK_STOPS));
  gl.uniform3fv(at("uBody"), flatten(frame.body, BODY_STOPS));
}

type Target = { texture: WebGLTexture; buffer: WebGLFramebuffer; side: number };

/**
 * The offscreen buffer the points add up in.
 *
 * Half-float where the browser can render to it, so the light can pass 1.0 and
 * the tonemap rolls it off. Eight bits otherwise: the sum clips but draws.
 */
function makeTarget(
  gl: WebGL2RenderingContext,
  side: number,
  halfFloat: boolean,
): Target | null {
  const texture = gl.createTexture();
  const buffer = gl.createFramebuffer();
  if (!texture || !buffer) {
    return null;
  }
  gl.bindTexture(gl.TEXTURE_2D, texture);
  if (halfFloat) {
    gl.texImage2D(
      gl.TEXTURE_2D,
      0,
      gl.RGBA16F,
      side,
      side,
      0,
      gl.RGBA,
      gl.HALF_FLOAT,
      null,
    );
  } else {
    gl.texImage2D(
      gl.TEXTURE_2D,
      0,
      gl.RGBA8,
      side,
      side,
      0,
      gl.RGBA,
      gl.UNSIGNED_BYTE,
      null,
    );
  }
  gl.texParameteri(
    gl.TEXTURE_2D,
    gl.TEXTURE_MIN_FILTER,
    gl.LINEAR_MIPMAP_LINEAR,
  );
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  gl.bindFramebuffer(gl.FRAMEBUFFER, buffer);
  gl.framebufferTexture2D(
    gl.FRAMEBUFFER,
    gl.COLOR_ATTACHMENT0,
    gl.TEXTURE_2D,
    texture,
    0,
  );
  const complete =
    gl.checkFramebufferStatus(gl.FRAMEBUFFER) === gl.FRAMEBUFFER_COMPLETE;
  gl.bindFramebuffer(gl.FRAMEBUFFER, null);
  if (!complete) {
    gl.deleteFramebuffer(buffer);
    gl.deleteTexture(texture);
    return null;
  }
  return { texture, buffer, side };
}

function dropTarget(gl: WebGL2RenderingContext, target: Target | null): void {
  if (target) {
    gl.deleteFramebuffer(target.buffer);
    gl.deleteTexture(target.texture);
  }
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
  const points = link(gl, CORE_POINTS_VERT, CORE_POINTS_FRAG);
  const composite = link(gl, CORE_VERT, CORE_FRAG);
  const vao = gl.createVertexArray();
  if (!points || !composite || !vao) {
    gl.deleteProgram(points);
    gl.deleteProgram(composite);
    gl.deleteVertexArray(vao);
    return null;
  }
  gl.bindVertexArray(vao);
  gl.disable(gl.DEPTH_TEST);
  const atPoints = locate(gl, points);
  const atComposite = locate(gl, composite);
  const halfFloat = gl.getExtension("EXT_color_buffer_float") !== null;
  let target: Target | null = null;

  const ensureTarget = (): Target | null => {
    const side = canvas.width;
    if (target?.side !== side) {
      dropTarget(gl, target);
      target = makeTarget(gl, side, halfFloat);
    }
    return target;
  };

  return {
    resize(cssSize, dpr) {
      const side = Math.max(1, Math.round(cssSize * Math.min(dpr, MAX_DPR)));
      if (canvas.width === side && canvas.height === side) {
        return;
      }
      canvas.width = side;
      canvas.height = side;
    },
    draw(frame) {
      const side = canvas.width;
      const into = ensureTarget();
      if (!into) {
        return;
      }
      const grid = pointGrid(side);
      gl.viewport(0, 0, side, side);

      gl.bindFramebuffer(gl.FRAMEBUFFER, into.buffer);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      select(gl, points);
      setFrame(gl, atPoints, frame, side);
      gl.uniform2i(atPoints("uGrid"), grid.along, grid.across);
      gl.uniform1f(atPoints("uPointPx"), grid.pointPx);
      gl.enable(gl.BLEND);
      gl.blendFuncSeparate(gl.ONE, gl.ONE, gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      gl.drawArrays(gl.POINTS, 0, LAYERS * grid.along * grid.across);
      gl.disable(gl.BLEND);

      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.clear(gl.COLOR_BUFFER_BIT);
      select(gl, composite);
      setFrame(gl, atComposite, frame, side);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, into.texture);
      gl.generateMipmap(gl.TEXTURE_2D);
      gl.uniform1i(atComposite("uParticles"), 0);
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
      dropTarget(gl, target);
      target = null;
      gl.deleteVertexArray(vao);
      gl.deleteProgram(points);
      gl.deleteProgram(composite);
    },
  };
}

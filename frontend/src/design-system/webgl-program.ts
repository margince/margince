// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The WebGL2 plumbing a full-screen shader needs: compile, link, report, bind.
 *
 * None of it decides what anything LOOKS like. It exists so a shader file can
 * be nothing but its own GLSL and a renderer file nothing but its own uniforms.
 *
 * **The Core keeps its own copy of compile and link** (`margince-core-gl.ts`)
 * and is deliberately not rewired onto this one. Two writers of one invariant
 * normally share a helper; the reason they do not here is that the orb is a
 * shipped surface whose GL setup has no bug, and touching it to save twenty
 * lines puts the product's one piece of AI identity at risk for no change a
 * reader could see. If the orb's context setup is opened again for its own
 * reasons, this is the file it should land on.
 *
 * Nothing here throws. A host without WebGL2, a driver that refuses to compile,
 * a context lost to a GPU reset: each is a real state on real machines, and the
 * answer to all three is for the caller to fall back to its static dress. So
 * these return null and say why in the console, rather than taking a page down
 * over an ornament.
 */

/**
 * The vertex shader for a full-viewport pass.
 *
 * Three vertices derived from `gl_VertexID`, so there is no buffer, no
 * attribute and nothing to upload: the caller binds an empty vertex array and
 * draws. A triangle rather than two triangles for a quad because the diagonal
 * seam of a quad is a real cost on tiled GPUs and buys nothing.
 */
export const FULLSCREEN_TRIANGLE_VERT = `#version 300 es
void main(){
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

/**
 * 4x4 ordered dither, as a GLSL fragment for a fragment shader to interpolate.
 *
 * A per-pixel hash also debands, but it is white noise: on thin high-contrast
 * structures that reads as grain. An ordered offset spreads the same error over
 * a fixed tile instead. Callers add `(bayer(gl_FragCoord.xy) - 0.5) / 255.0` to
 * the colour on output.
 */
export const GLSL_BAYER = `
float bayer(vec2 f){
  const float B[16] = float[16](
     0.0,  8.0,  2.0, 10.0,
    12.0,  4.0, 14.0,  6.0,
     3.0, 11.0,  1.0,  9.0,
    15.0,  7.0, 13.0,  5.0);
  int x = int(mod(f.x, 4.0));
  int y = int(mod(f.y, 4.0));
  return (B[y * 4 + x] + 0.5) / 16.0;
}`;

function compile(
  gl: WebGL2RenderingContext,
  type: number,
  source: string,
  label: string,
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
  console.error(`${label} shader failed to compile`, {
    log: gl.getShaderInfoLog(shader),
  });
  gl.deleteShader(shader);
  return null;
}

/**
 * Links one full-screen program, or returns null with the reason logged.
 *
 * `label` names the surface in the console, because two shaders that fail the
 * same way on the same driver are otherwise indistinguishable in a bug report.
 */
export function createFullscreenProgram(
  gl: WebGL2RenderingContext,
  fragment: string,
  label: string,
): WebGLProgram | null {
  const vert = compile(
    gl,
    gl.VERTEX_SHADER,
    FULLSCREEN_TRIANGLE_VERT,
    `${label} vertex`,
  );
  const frag = compile(gl, gl.FRAGMENT_SHADER, fragment, `${label} fragment`);
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
  console.error(`${label} program failed to link`, {
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
export function bindFullscreenProgram(
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

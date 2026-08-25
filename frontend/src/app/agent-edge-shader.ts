// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The lit edge, as a fragment shader.
 *
 * This replaced a masked-band-plus-SVG-filter build that could not do the two
 * things the design asks for. It could not make the crests TRAVEL, because a
 * turbulence field cannot be slid along a perimeter without popping when the loop
 * comes round; and it could not stay cheap, because it cost a full-viewport
 * turbulence pass, two gaussian blurs and fifteen animated blurred elements every
 * frame, which is enough to make a fullscreen window stutter.
 *
 * Here both are straightforward. The wave is a sum of sines whose phase is a
 * function of distance ALONG the edge, so it travels by construction and loops
 * seamlessly because a sine does. The rim's boundary is a `smoothstep` across one
 * pixel rather than a displaced raster, so it is smooth by construction too, at
 * any size and any device ratio.
 */

export const EDGE_VERT = `#version 300 es
in vec2 aPos;
void main() {
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`;

export const EDGE_FRAG = `#version 300 es
precision highp float;

out vec4 outColor;

uniform vec2  uRes;      // drawing-buffer size, in device pixels
uniform float uDpr;      // device pixels per CSS pixel, so widths stay in CSS px
uniform float uTime;     // seconds since the edge lit
uniform float uLevel;    // 0..1 fade in and out, so nothing appears or cuts
uniform float uThick;    // the rim's resting thickness, CSS px
uniform vec3  uHueA;     // cool
uniform vec3  uHueB;
uniform vec3  uHueC;
uniform vec3  uHueD;
uniform vec3  uHueE;     // warm

/**
 * Distance to the nearest edge, and the distance ALONG that edge.
 *
 * The second one is what makes travelling waves possible: it is the perimeter
 * unrolled into a single coordinate, so a phase that advances with it moves
 * around the frame instead of pulsing in place. The four sides are laid end to
 * end, each offset by the length of the ones before it, so the phase is
 * continuous through a corner rather than restarting there.
 */
void edgeCoords(vec2 p, vec2 size, out float dist, out float along) {
  float dL = p.x;
  float dR = size.x - p.x;
  float dT = p.y;
  float dB = size.y - p.y;
  dist = min(min(dL, dR), min(dT, dB));
  if (dist == dT) {
    along = p.x;                                   // top, left to right
  } else if (dist == dR) {
    along = size.x + p.y;                          // right, top to bottom
  } else if (dist == dB) {
    along = size.x + size.y + (size.x - p.x);      // bottom, right to left
  } else {
    along = size.x + size.y + size.x + (size.y - p.y);
  }
}

/** Three trains at unrelated wavelengths and speeds, two of them travelling
 *  against the other. Their sum never repeats inside a period anybody watches,
 *  which is the difference between a wave and a rhythm. */
float waves(float along, float t) {
  // Shorter wavelengths than the first cut, which is what "more wavy" asks for:
  // waviness is crests PER LENGTH of rim, not how far each one swells. A long
  // wave across a whole side reads as the edge breathing; these put several
  // crests on every side.
  // The high harmonics carry the roughness, so they are the ones to hold back:
  // each term above the first is weighted well under the one before it, which is
  // what keeps the sum a rolling line rather than a busy one. Same number of
  // crests per side, softer shoulders on each.
  float w = sin(along * 0.0255 - t * 1.15);
  w += 0.62 * sin(along * 0.0430 + t * 0.83);
  w += 0.28 * sin(along * 0.0690 - t * 1.75);
  w += 0.11 * sin(along * 0.1050 + t * 2.30);
  return w / 2.01;
}

/** The hues, cool to warm and back, so the loop closes without a seam. */
vec3 palette(float x) {
  float p = fract(x) * 5.0;
  if (p < 1.0) return mix(uHueA, uHueB, smoothstep(0.0, 1.0, p));
  if (p < 2.0) return mix(uHueB, uHueC, smoothstep(0.0, 1.0, p - 1.0));
  if (p < 3.0) return mix(uHueC, uHueD, smoothstep(0.0, 1.0, p - 2.0));
  if (p < 4.0) return mix(uHueD, uHueE, smoothstep(0.0, 1.0, p - 3.0));
  return mix(uHueE, uHueA, smoothstep(0.0, 1.0, p - 4.0));
}

void main() {
  // Work in CSS pixels throughout: every width below is a width a designer
  // chose, and dividing here is what keeps it that width on a 2x display.
  vec2 p = gl_FragCoord.xy / uDpr;
  vec2 size = uRes / uDpr;

  float dist;
  float along;
  edgeCoords(p, size, dist, along);

  float t = uTime;
  float w = waves(along, t);

  // The rim's thickness undulates with the wave, so the crests are visible as
  // the edge swelling and thinning rather than as a stripe moving inside it.
  // Amplitude, and it is generous on purpose: the rim nearly doubles at a crest
  // and thins to well under its resting width in a trough, which is what makes
  // the wave legible on something only a few pixels across.
  float thick = uThick * (1.0 + 0.78 * w) + 1.0;

  // The whole reason this is a shader: one pixel of smoothstep across the
  // boundary, computed per fragment. There is no raster to displace and nothing
  // to alias, so the edge is as smooth at 4K as at 1x.
  float aa = 1.0 + 0.6 * fwidth(dist);
  float core = 1.0 - smoothstep(thick - aa, thick + aa, dist);

  // Two halos, both exponential falloffs rather than blurs: the near one seats
  // the rim, the far one is the light it throws onto the page. An exponential is
  // what a real falloff looks like and costs two instructions.
  float near = exp(-max(dist - thick, 0.0) / (7.0 + 5.0 * w));
  float far = exp(-max(dist - thick, 0.0) / 46.0);

  // The gradient MOVES inside the waves, and slower than they do: hue drifting
  // at the wave's own speed would read as one object sliding past.
  //
  // Pulled most of the way back toward jade. The full spread from teal to gold
  // was doing too much: an edge that runs the palette reads as a decoration in
  // its own right, and this one has a job. A third of the way out keeps the
  // shift visible as the light travels without the frame becoming the subject.
  vec3 tint = mix(uHueB, palette(along / 1850.0 + t * 0.045), 0.34);

  float alpha = (core * 0.95 + near * 0.34 + far * 0.13) * uLevel;
  alpha = clamp(alpha, 0.0, 1.0);

  // Premultiplied, because this canvas composites over live application UI: a
  // straight-alpha edge fringes dark against a light page.
  outColor = vec4(tint * alpha, alpha);
}
`;

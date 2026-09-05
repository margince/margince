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
uniform float uWave;     // how much the rim breathes: 1 as tuned, less for an import
uniform float uBeam;     // the head's share: the register's, 0 under reduced motion
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

float hash11(float n) {
  return fract(sin(n * 17.13) * 43758.5453123);
}

/**
 * Value noise that is PERIODIC over the whole rim.
 *
 * u is turns around the perimeter, so the wrap has to be exact rather than
 * merely continuous: a field that only looks seamless leaves a discontinuity
 * where the last side meets the first, and that seam then sits in one corner of
 * a reader screen for as long as the app is open. Taking the cell index modulo
 * the cell count makes the last cell interpolate back into the first by
 * construction.
 */
float pnoise(float u, float cells, float seed) {
  float x = u * cells;
  float i = floor(x);
  float f = fract(x);
  float a = hash11(mod(i, cells) + seed);
  float b = hash11(mod(i + 1.0, cells) + seed);
  f = f * f * (3.0 - 2.0 * f);
  return mix(a, b, f) * 2.0 - 1.0;
}

/**
 * The rim displacement: warped fractal noise rather than a sum of sines.
 *
 * Tuning could not get the sine version here, and the reason is structural. A
 * sum of sines spaces its crests EVENLY however many terms it has, because
 * every term is symmetric about its own zero crossings, and evenly spaced
 * crests are what read as machined. Detuning the wavelengths delays the moment
 * a reader recognises the pattern; it does not change the texture.
 *
 * Noise has no such symmetry. Crests arrive in clusters, with quiet stretches
 * between them, at widths that vary along the rim.
 */
float waves(float u, float t) {
  // Domain warp: the field is read through a slow distortion of its own
  // coordinate. This does the most work here, and it is the thing a sine sum has
  // no equivalent for: crests stretch and bunch as the warp slides, so the
  // SPACING is in motion rather than only the crests.
  float warp = 0.055 * pnoise(u - t * 0.021, 3.0, 11.0)
             + 0.028 * pnoise(u + t * 0.017, 5.0, 27.0);
  float q = u + warp;

  // Coprime cell counts, each octave scrolling at its own speed and direction,
  // so the layers slide over one another and the field never returns to a state
  // it has already been in.
  float w = 1.00 * pnoise(q - t * 0.030, 7.0, 3.0);
  w += 0.55 * pnoise(q + t * 0.019, 13.0, 41.0);
  w += 0.30 * pnoise(q - t * 0.047, 23.0, 77.0);
  w += 0.16 * pnoise(q + t * 0.062, 41.0, 129.0);
  w /= 2.01;

  // The swell is noise too. A sine envelope puts the calm stretch in a
  // predictable place and moves it at a constant rate, which is the same tell
  // one level up.
  //
  // Clamped both sides, and that bound is load-bearing: everything downstream
  // that divides by a wave-dependent quantity is safe only while w stays inside
  // about plus or minus one. A halo reach that crosses zero turns its falloff
  // into a blowup and draws the crests as full-viewport bars.
  float swell = 0.62 + 0.52 * pnoise(u - t * 0.013, 2.0, 5.0);
  return w * clamp(swell, 0.22, 1.15);
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
  float peri = 2.0 * (size.x + size.y);
  float w = waves(along / peri, t);

  // One head of light making a lap, shaped like a comet rather than a blob.
  //
  // The waves say the agent is working; the beam says it is still working NOW,
  // which a standing pattern cannot: a reader who glances twice at a loop has no
  // way to tell it from a still image.
  //
  // Asymmetric on purpose. A symmetric falloff reads as a lamp being carried
  // around the frame, while a short bright nose with a long tail behind it reads
  // as something travelling, because the tail says where it has been. The offset
  // is SIGNED for that, taken the short way round so the head crosses the seam
  // between the last side and the first without thinning and reappearing.
  float head = fract(t * 0.205) * peri;
  float rel = along - head;
  rel -= peri * floor(rel / peri + 0.5);
  float nose = peri * 0.030;
  float tail = peri * 0.150;
  // Each exponential is evaluated over the half it governs and flat over the
  // other, so neither is handed a positive argument. Written this way rather
  // than as a branch or a mix because exp() of a large positive is inf, and an
  // inf reaching a mix() that would have discarded it still yields NaN.
  float back = exp(min(rel, 0.0) / tail);
  float front = exp(-max(rel, 0.0) / nose);
  // Scaled to nothing under reduced motion, and that is a correctness fix rather
  // than a preference. The loop holds t at zero to stop the motion, which pins
  // the head at along == 0: the comet does not disappear, it PARKS in one corner
  // and burns there, thickening the rim and brightening the halo at that corner
  // for as long as the edge is lit. A reader who asked for less movement was
  // getting a permanent asymmetric hotspot instead of an even rim.
  float beam = min(back, front) * uBeam;

  // The rim's thickness undulates with the wave, so the crests are visible as
  // the edge swelling and thinning rather than as a stripe moving inside it.
  // Amplitude, and it is generous on purpose: at full breath the rim nearly
  // doubles at a crest and thins to well under its resting width in a trough,
  // which is what makes the wave legible on something only a few pixels across.
  // uWave scales that breath, and the halo's with it below: it is the one dial
  // that turns the whole picture calmer without turning it off.
  float swell = w * uWave;
  float thick = uThick * max(0.25, 1.0 + 1.15 * swell + 0.85 * beam) + 1.0;

  // The whole reason this is a shader: one pixel of smoothstep across the
  // boundary, computed per fragment. There is no raster to displace and nothing
  // to alias, so the edge is as smooth at 4K as at 1x.
  float aa = 1.0 + 0.6 * fwidth(dist);
  float core = 1.0 - smoothstep(thick - aa, thick + aa, dist);

  // Two halos, both exponential falloffs rather than blurs: the near one seats
  // the rim, the far one is the light it throws onto the page. An exponential is
  // what a real falloff looks like and costs two instructions.
  float reach = mix(4.0, 22.0, clamp(0.5 + 0.5 * swell, 0.0, 1.0));
  float near = exp(-max(dist - thick, 0.0) / reach);
  float far = exp(-max(dist - thick, 0.0) / 46.0);
  // A calmer rim throws less light onto the page. Not none: the halo is what
  // seats the rim against the window, and a thin rim with no seat reads as a
  // hairline border rather than as light.
  float glow = mix(0.55, 1.0, clamp(uWave, 0.0, 1.0));

  // The gradient MOVES inside the waves, and slower than they do: hue drifting
  // at the wave's own speed would read as one object sliding past.
  //
  // Pulled most of the way back toward jade. The full spread from teal to gold
  // was doing too much: an edge that runs the palette reads as a decoration in
  // its own right, and this one has a job. A third of the way out keeps the
  // shift visible as the light travels without the frame becoming the subject.
  vec3 tint = mix(uHueB, palette(along / 1850.0 + t * 0.095), 0.34);
  // The head is the palette's light end, so the beam reads as MORE LIGHT rather
  // than as a different colour arriving.
  tint = mix(tint, uHueC, 0.55 * beam);

  float alpha = (core * 0.95
                 + (near * (0.34 + 0.30 * beam) + far * (0.13 + 0.10 * beam)) * glow)
              * uLevel;
  alpha = clamp(alpha, 0.0, 1.0);

  // Premultiplied, because this canvas composites over live application UI: a
  // straight-alpha edge fringes dark against a light page.
  outColor = vec4(tint * alpha, alpha);
}
`;

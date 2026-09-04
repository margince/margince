// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { GLSL_BAYER } from "./webgl-program";

/**
 * The welcome ground, as GLSL: slow colour bands folding through each other.
 *
 * WHAT THE PICTURE IS. Three ribbons of light over the page's own paper colour,
 * widest and palest first. Each is a soft window on a field of simplex noise
 * (so it has no edge anywhere) with one broad highlight down its spine to give
 * it a surface. The middle of the screen clears back to paper, because the copy
 * and the form live there.
 *
 * WHAT MAKES IT A RIBBON RATHER THAN A PATCH is the warp: the field is sampled
 * at a point that a second, slower field has already displaced, which drags the
 * shapes out along a direction and folds them. Without it this is three clouds;
 * with too much of it, marbling. See WARP below.
 *
 * WHY PER-PIXEL AND NOT A DEFORMED MESH. The effect this is drawn from displaces
 * a segmented plane in the vertex shader and colours the result. Sampling the
 * same noise per fragment gives the same picture with no geometry at all: no
 * buffers, no index arrays, no re-tessellation on resize, and the ribbons stay
 * smooth at any size instead of being as smooth as the mesh was dense.
 *
 * WHY IT SAYS NOTHING. Provenance colour is a claim (`frontend/AGENTS.md`), and
 * this surface must not make one. Three things keep it clear of that: the hues
 * arrive already mixed most of the way back to the page's paper, so no band is
 * ever the token's own saturated tone; nothing here has an edge, a shape or a
 * position that could be pointed at; and it moves too slowly for any of it to be
 * watched happening. It is the light in the room, not a thing in the room.
 */
export const WAVES_FRAG = `#version 300 es
precision highp float;
out vec4 outColor;

uniform vec2  iResolution;
uniform float uTime;    // seconds, accumulated host-side so a pause cannot jump
uniform float uFade;    // 0..1 arrival: the ground comes up rather than snapping
uniform vec3  uPaper;   // the surface underneath, so the bands sit ON the page
uniform vec3  uHue[3];  // the band palette, read off the tokens

const int NBAND = 3;

/* How far the warp displaces the field before it is read. This one number is
   the difference between noise and silk: at zero the bands are the blobs the
   noise happens to have, and pushed too far they fold over themselves into
   marbling. */
const float WARP = 0.52;

/* How far each band's BODY travels from the paper towards its own hue.
   NOWHERE NEAR the whole way, and that is the difference between weather and
   camouflage: at half strength three hues read as three patches with borders
   between them, and the eye finds the borders. At a third they read as one
   surface being lit unevenly. The structure comes from the crest below, not
   from making the bodies stronger. */
const float REACH[3] = float[3](0.34, 0.26, 0.19);
/* Strongly anisotropic (low across, higher down), which is what makes a band a
   long sweep rather than a blob. The ratio matters more than either number:
   bring the two together and the field goes back to patches. */
const vec2 FREQ[3] = vec2[3](vec2(0.44, 1.30), vec2(0.58, 1.70), vec2(0.74, 2.15));
const float FLOW[3] = float[3](0.042, 0.055, 0.068);   // travel along the band
const float ROLL[3] = float[3](0.013, 0.018, 0.023);   // the field's own churn
const float SEED[3] = float[3](0.0, 17.0, 41.0);
/* The body's window: still wide, so the pale wash under everything has no place
   a reader can say it begins. */
const float LO[3] = float[3](0.26, 0.30, 0.34);
const float HI[3] = float[3](0.78, 0.74, 0.70);
/*
 * THE SPINE: a soft highlight down the middle of each ribbon.
 *
 * A flat wash has no surface. The spine is where the field peaks, and tinting
 * it further than the body gives the ribbon somewhere to catch light, which is
 * what stops it reading as a coloured shape.
 *
 * CREST is the level it sits at and BLADE how far it reaches either side. BLADE
 * is LARGE on purpose: a narrow one turns the highlight into a contour line,
 * and a screen of contour lines is a topographic map rather than weather. GLINT
 * is how far past its own body's reach the highlight goes.
 */
const float CREST[3] = float[3](0.70, 0.66, 0.62);
const float BLADE[3] = float[3](0.20, 0.18, 0.16);
const float GLINT[3] = float[3](0.46, 0.38, 0.30);

vec3 mod289(vec3 x){ return x - floor(x * (1.0 / 289.0)) * 289.0; }
vec4 mod289(vec4 x){ return x - floor(x * (1.0 / 289.0)) * 289.0; }
vec4 permute(vec4 x){ return mod289(((x * 34.0) + 1.0) * x); }
vec4 taylorInvSqrt(vec4 r){ return 1.79284291400159 - 0.85373472095314 * r; }

/* Simplex noise in three dimensions: two for the picture, one for time, which
   is what lets the field evolve in place instead of only sliding past. */
float snoise(vec3 v){
  const vec2 C = vec2(1.0 / 6.0, 1.0 / 3.0);
  const vec4 D = vec4(0.0, 0.5, 1.0, 2.0);

  vec3 i  = floor(v + dot(v, C.yyy));
  vec3 x0 = v - i + dot(i, C.xxx);

  vec3 g = step(x0.yzx, x0.xyz);
  vec3 l = 1.0 - g;
  vec3 i1 = min(g.xyz, l.zxy);
  vec3 i2 = max(g.xyz, l.zxy);

  vec3 x1 = x0 - i1 + C.xxx;
  vec3 x2 = x0 - i2 + C.yyy;
  vec3 x3 = x0 - D.yyy;

  i = mod289(i);
  vec4 p = permute(permute(permute(
             i.z + vec4(0.0, i1.z, i2.z, 1.0))
           + i.y + vec4(0.0, i1.y, i2.y, 1.0))
           + i.x + vec4(0.0, i1.x, i2.x, 1.0));

  float n_ = 0.142857142857;
  vec3 ns = n_ * D.wyz - D.xzx;

  vec4 j = p - 49.0 * floor(p * ns.z * ns.z);
  vec4 x_ = floor(j * ns.z);
  vec4 y_ = floor(j - 7.0 * x_);

  vec4 x = x_ * ns.x + ns.yyyy;
  vec4 y = y_ * ns.x + ns.yyyy;
  vec4 h = 1.0 - abs(x) - abs(y);

  vec4 b0 = vec4(x.xy, y.xy);
  vec4 b1 = vec4(x.zw, y.zw);
  vec4 s0 = floor(b0) * 2.0 + 1.0;
  vec4 s1 = floor(b1) * 2.0 + 1.0;
  vec4 sh = -step(h, vec4(0.0));

  vec4 a0 = b0.xzyw + s0.xzyw * sh.xxyy;
  vec4 a1 = b1.xzyw + s1.xzyw * sh.zzww;

  vec3 p0 = vec3(a0.xy, h.x);
  vec3 p1 = vec3(a0.zw, h.y);
  vec3 p2 = vec3(a1.xy, h.z);
  vec3 p3 = vec3(a1.zw, h.w);

  vec4 norm = taylorInvSqrt(vec4(dot(p0, p0), dot(p1, p1), dot(p2, p2), dot(p3, p3)));
  p0 *= norm.x; p1 *= norm.y; p2 *= norm.z; p3 *= norm.w;

  vec4 m = max(0.6 - vec4(dot(x0, x0), dot(x1, x1), dot(x2, x2), dot(x3, x3)), 0.0);
  m = m * m;
  return 42.0 * dot(m * m, vec4(dot(p0, x0), dot(p1, x1), dot(p2, x2), dot(p3, x3)));
}
${GLSL_BAYER}
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = frag / iResolution;

  /* Aspect-corrected across, so a band is the same thickness on a phone as on a
     wide display rather than being stretched by the viewport's shape. */
  vec2 p = vec2(uv.x * (iResolution.x / max(iResolution.y, 1.0)), uv.y);
  /* The set of the whole picture: the bands run uphill across the screen. Level
     bands read as a striped background; tilted ones read as weather. */
  p.y += 0.18 * p.x;

  /*
   * THE WARP, and it is the whole picture.
   *
   * Reading noise directly gives whatever shapes that noise happens to have,
   * which is patches. Displacing the sample point by a second, slower field
   * first (domain warping) drags those shapes out along a direction, and what
   * comes back is a ribbon with a length, a fold and a consistent flow. That is
   * the difference between a texture and something that looks poured.
   *
   * Two fields, not one, because a single displacement moves every point the
   * same way and only slides the picture sideways.
   */
  float wx = snoise(vec3(p.x * 0.85, p.y * 1.30, uTime * 0.031));
  float wy = snoise(vec3(p.x * 1.10 + 4.7, p.y * 1.60, uTime * 0.024 + 9.3));
  vec2 q = p + WARP * vec2(wx, wy);

  /*
   * HOW FAR THE HUES ARE ALLOWED TO GO, on this page's paper.
   *
   * The reaches below are tuned against near-white. The same numbers on the
   * dark theme's paper are a far bigger step in lightness: the ribbons stopped
   * being atmosphere and became the loudest thing on the screen, brighter than
   * the copy in front of them. So the whole palette is scaled by how light the
   * paper is: on paper it reaches its tuned distance, on a dark page about two
   * thirds of it. Two thirds and not half, which is where this started: half
   * was quiet enough to be nearly absent, and a dark page has more room for
   * light on it than a white one, not less.
   */
  float lum = dot(uPaper, vec3(0.2126, 0.7152, 0.0722));
  float gain = mix(0.66, 1.0, smoothstep(0.10, 0.72, lum));

  vec3 col = uPaper;
  for(int i = 0; i < NBAND; i++){
    float n = snoise(vec3(
      q.x * FREQ[i].x + uTime * FLOW[i],
      q.y * FREQ[i].y,
      uTime * ROLL[i] + SEED[i]
    )) * 0.5 + 0.5;
    /* The ribbon's body. Squared and no further: a higher power was what made
       three washes read as three separate clouds. */
    float body = smoothstep(LO[i], HI[i], n);
    col = mix(col, mix(uPaper, uHue[i], REACH[i] * gain), body * body);

    /* One highlight down the ribbon's spine, where the field peaks. It is what
       gives a flat wash a surface to catch light on, and it is deliberately
       WIDE and weak rather than a line: a narrow one drew a contour map. */
    float spine = smoothstep(CREST[i] - BLADE[i], CREST[i], n)
                * (1.0 - smoothstep(CREST[i], CREST[i] + BLADE[i], n));
    col = mix(col, mix(uPaper, uHue[i], GLINT[i] * gain), spine * 0.55);
  }

  /*
   * THE CLEARING.
   *
   * The copy and the form sit in the middle of this screen, and no amount of
   * subtlety in the ribbons makes a wave passing behind a password field
   * anything other than something to read past. So the middle is paper, and
   * the ribbons live in the margins where they are all a reader ever sees of
   * them. It is offset upward because the composition is: orb, greeting, form.
   */
  float clearing = length((uv - vec2(0.5, 0.46)) * vec2(1.00, 1.55));
  col = mix(uPaper, col, smoothstep(0.20, 0.78, clearing));

  /* And the corners settle back too, so the ribbons do not run into the edge of
     the viewport, which reads as an image placed on the page rather than as
     the page being lit. */
  float d = length((uv - 0.5) * vec2(1.10, 1.00));
  col = mix(col, uPaper, smoothstep(0.52, 1.02, d) * 0.45);

  /* The ground arrives with the rest of the screen rather than being there
     before it: the surface's paper is what it fades up from, so there is no
     frame in which anything is missing. */
  col = mix(uPaper, col, clamp(uFade, 0.0, 1.0));

  /* Eight bits over a gradient this wide bands visibly; the ordered offset
     spreads the error instead. */
  col += (bayer(frag) - 0.5) / 255.0;

  outColor = vec4(max(col, 0.0), 1.0);
}`;

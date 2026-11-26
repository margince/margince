// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The Core's shading, as GLSL, in its own file because it is a second language
 * beside the TypeScript engine (`margince-core-engine.ts`).
 *
 * Two passes. `CORE_POINTS_*` draws four ribbons of glowing points, additively,
 * into an offscreen buffer the size of the canvas. `CORE_FRAG` then reads that
 * buffer through a glass lens, one tap per channel for the chromatic split. It
 * lays that over the ball's body and adds the rim lights. The ball composites
 * with alpha onto whatever hosts it: no page background.
 */

/** Fullscreen triangle from the vertex id. No attribute buffers to bind. */
export const CORE_VERT = `#version 300 es
void main(){
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

/**
 * One point per vertex id, laid out as `layers x uGrid.x x uGrid.y`: along
 * each ribbon, then across it. No buffers, so a size change only changes the
 * draw count.
 */
export const CORE_POINTS_VERT = `#version 300 es
precision highp float;
precision highp int;

uniform vec2  iResolution;
uniform ivec2 uGrid;       // points along and across one ribbon
uniform float uPointPx;    // point diameter in device pixels
uniform float uPhase;
uniform float uForm;
uniform float uLevel;
uniform float uPulse;
uniform float uIngest;
uniform float uTint;
uniform vec3  uTintCol;
uniform vec3  uWork[5];

out vec3  vColor;
out float vAlpha;

const float TAU = 6.28318531;
const float LAYERS = 4.0;
/* the ball's silhouette radius in uv; the composite's camera sets it */
const float BALL = 0.3140;

mat3 rotY(float a){ float s=sin(a),c=cos(a); return mat3(c,0,-s, 0,1,0, s,0,c); }
mat3 rotX(float a){ float s=sin(a),c=cos(a); return mat3(1,0,0, 0,c,s, 0,-s,c); }

float unit(uint n){
  n ^= n >> 16; n *= 0x7feb352du;
  n ^= n >> 15; n *= 0x846ca68bu;
  n ^= n >> 16;
  return float(n >> 8) / 16777216.0;
}

/* The ribbon colours as one loop, A to D. Tokens are sRGB and the light adds
   up linearly. */
vec3 tone(float x){
  float k = fract(x) * 4.0;
  vec3 a = uWork[0], b = uWork[1], c = uWork[2], d = uWork[3];
  vec3 col = k < 1.0 ? mix(a, b, k)
           : k < 2.0 ? mix(b, c, k - 1.0)
           : k < 3.0 ? mix(c, d, k - 2.0)
           :           mix(d, a, k - 3.0);
  return pow(col, vec3(2.2));
}

void hide(){
  gl_Position = vec4(2.0, 2.0, 2.0, 1.0);
  gl_PointSize = 0.0;
  vColor = vec3(0.0);
  vAlpha = 0.0;
}

void main(){
  int perLayer = uGrid.x * uGrid.y;
  int layer = gl_VertexID / perLayer;
  int rest = gl_VertexID - layer * perLayer;
  int along = rest / uGrid.y;
  int across = rest - along * uGrid.y;
  float L = float(layer);
  uint seed = uint(gl_VertexID) * 3u;

  float t = uPhase * 0.4;
  /* the grain streams along its ribbon, faster the more energy is in it */
  float u = fract((float(along) + 0.6 * unit(seed + 1u)) / float(uGrid.x) + 0.16 * uLevel * t);
  float v = (float(across) + 0.5 * unit(seed + 2u)) / float(uGrid.y) * 2.0 - 1.0;

  /* each ribbon shows most of its loop and fades in and out at the ends */
  float arcLen = 0.76 + 0.05 * sin(0.23 * t + 1.7 * L);
  float arc = fract(u + 0.21 * L - 0.012 * t);
  if(arc > arcLen){ hide(); return; }
  float s = arc / arcLen;
  float ends = smoothstep(0.0, 0.08, s) * (1.0 - smoothstep(0.86, 1.0, s));

  float twist = mix(0.48, 1.15, uForm);
  float fold  = mix(0.21, 0.60, uForm);
  float halfW = mix(0.36, 0.57, uForm);
  float breath = mix(0.07, 0.38, uForm) * uPulse;

  /* the centre line: a loop round the vertical axis, tilted once and folded
     twice per lap, its radius pinched in step with the fold */
  float phi = TAU * u;
  float fp = 2.0 * phi + 0.5 * t + 2.1 * L;
  float lat = 0.16 * sin(phi + 1.3 * L) + (0.36 + 0.30 * fold) * sin(fp)
            + 0.06 * sin(3.0 * phi - 0.23 * t + 0.7 * L);
  float az = phi + 0.06 * t + 0.2 * fold * sin(phi - 0.11 * t + L);
  float r = 0.58 + (0.07 + 0.08 * fold) * cos(fp);
  r *= 1.0 + 0.16 * breath * (sin(0.37 * t + 1.7 * L) + 0.45 * sin(0.59 * t + 0.8 * L + 1.2));

  vec3 radial = vec3(cos(lat) * cos(az), sin(lat), cos(lat) * sin(az));
  vec3 up = vec3(-sin(lat) * cos(az), cos(lat), -sin(lat) * sin(az));
  /* the twist turns the band from upright to flat and back */
  float tw = phi * (0.7 + 0.6 * twist) + 0.37 * t + 1.17 * L;
  vec3 across3 = up * cos(tw) + radial * sin(tw);
  float width = halfW * (0.72 + 0.28 * sin(1.5 * phi + 0.5 * t + L) * sin(1.5 * phi + 0.5 * t + L))
              * mix(0.45, 1.0, sqrt(ends));
  vec3 p = radial * r + across3 * (v * width);

  float lc = L - 0.5 * (LAYERS - 1.0);
  p = rotX(0.18 * lc + 0.04 * cos(0.17 * t + 0.9 * L)) * rotY(0.36 * lc + 0.05 * sin(0.19 * t + 1.3 * L)) * p;
  p = rotX(0.36 + 0.12 * sin(0.16 * t)) * rotY(0.10 * t + 0.11 * sin(0.21 * t)) * p;
  p *= 0.94;

  /* Ingest: a share of the grain leaves its ribbon and streams to the core,
     each point on its own cycle, fading in at the ribbon and out at the core.
     On a small ball fewer, larger and brighter motes fall, so the fall can be
     counted rather than read as haze. */
  float fade = 1.0;
  float mote = 0.0;
  float small = clamp(240.0 / iResolution.y, 1.0, 3.0);
  if(uIngest > 0.001 && unit(seed + 5u) < 0.55 * uIngest / (small * small * small)){
    float fall = fract(unit(seed + 4u) + uPhase * 0.3);
    p *= 1.0 - 0.9 * fall;
    fade = smoothstep(0.0, 0.15, fall) * (1.0 - smoothstep(0.7, 1.0, fall));
    mote = small - 1.0;
  }

  vec2 uv = p.xy * BALL * (1.0 + 0.07 * p.z);
  gl_Position = vec4(uv * 2.0 * iResolution.y / iResolution, 0.5 - 0.1 * p.z, 1.0);
  gl_PointSize = uPointPx * (0.9 + 0.18 * p.z) * (1.0 + 0.9 * mote);

  float edge = smoothstep(0.55, 1.0, abs(v));
  float front = clamp(0.78 + 0.5 * p.z, 0.45, 1.25);
  vec3 col = tone(0.32 * u + 0.19 * L + 0.012 * t + 0.08 * p.z);
  col = mix(col, mix(pow(uWork[4], vec3(2.2)), vec3(1.0), 0.35), 0.6 * edge);
  float luma = dot(col, vec3(0.2126, 0.7152, 0.0722));
  col = mix(vec3(luma), col, mix(0.95, 1.15, uForm));
  /* the warm stop is in the palette, so a stopped state's tint has to all but
     replace it to stay unmistakable */
  col = mix(col, uTintCol, uTint * 0.92);
  col = mix(col, vec3(1.0), min(0.45 * mote, 0.6));
  vColor = col * front;

  /* light per point falls as the points per pixel rise, so a rail orb and a
     hero carry the same brightness with very different counts */
  float overlap = float(perLayer) * uPointPx * uPointPx / (iResolution.x * iResolution.y);
  vAlpha = 0.045 / max(overlap, 0.05) * mix(1.1, 1.0, uForm) * (0.4 + 1.9 * edge) * ends * fade * (unit(seed + 3u) * 0.6 + 0.7)
         * (1.0 + 1.5 * mote);
}`;

export const CORE_POINTS_FRAG = `#version 300 es
precision highp float;
in vec3  vColor;
in float vAlpha;
out vec4 outColor;

void main(){
  vec2 d = gl_PointCoord * 2.0 - 1.0;
  float r2 = dot(d, d);
  if(r2 > 1.0) discard;
  /* a hot core in a soft bloom */
  float a = vAlpha * (exp(-4.6 * r2) * 1.9 + exp(-1.4 * r2) * 0.3);
  outColor = vec4(vColor * a, a);
}`;

export const CORE_FRAG = `#version 300 es
precision highp float;
out vec4 outColor;

/* How much of the ball's body a dark page gets, and how much of its bloom. */
#define DARKBODY 0.62
#define DARKGLOW 0.55

uniform vec2  iResolution;
uniform sampler2D uParticles;
uniform float uLevel;      // 0..1 exposure and core heat
uniform float uForm;       // 0 = the calm idle figure, 1 = the thinking figure
uniform float uPhase;      // integrated host-side, so a speed change bends the
                           // motion instead of teleporting it
uniform float uTint;       // 0..1 how far the palette collapses to uTintCol
uniform vec3  uTintCol;
uniform float uIngest;     // 0..1 the inward "pulling material in" sweep
uniform float uPaper;      // 0 = emissive on a dark surface, 1 = opaque on paper
uniform vec3  uWork[5];    // the ribbon palette, read off the tokens
uniform vec3  uBody[2];    // the ball's own base gradient, read off the tokens

const float R  = 1.0;
const float PI = 3.14159265;
const float TAU = 6.28318531;

const vec3 LUMA = vec3(0.2126, 0.7152, 0.0722);

vec3 linear(vec3 c){ return pow(c, vec3(2.2)); }

/* The lens squeezes the rim, so the buffer is read through its mipmaps, and
   blurrier the deeper the squeeze: point grain read through the three channel
   taps would otherwise speckle in colour instead of fringing. */
vec3 lightAt(vec2 q, float sr, float bend){
  float bias = 2.4 * bend - 0.6 * smoothstep(60.0, 200.0, sr * iResolution.y);
  return texture(uParticles, (q * sr * iResolution.y + 0.5 * iResolution) / iResolution, bias).rgb;
}

/* a lobe of light on the rim, centred on one direction */
float lobe(vec2 n, vec2 toward, float cut, float sharp){
  return pow(clamp((dot(n, toward) - cut) / (1.0 - cut), 0.0, 1.0), sharp);
}

/* 4x4 ordered dither: 8-bit banding is very visible on this palette, and an
   ordered offset debands without adding grain of its own. */
float bayer(vec2 f){
  const float B[16] = float[16](
     0.0,  8.0,  2.0, 10.0,
    12.0,  4.0, 14.0,  6.0,
     3.0, 11.0,  1.0,  9.0,
    15.0,  7.0, 13.0,  5.0);
  int x = int(mod(f.x, 4.0));
  int y = int(mod(f.y, 4.0));
  return (B[y * 4 + x] + 0.5) / 16.0;
}

void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - 0.5 * iResolution) / iResolution.y;
  float ta = uPhase;

  /* the camera sits 3.2 radii from a unit ball behind a 1.05 lens, so the
     silhouette's radius in uv is tan(asin(1 / 3.2)) / 1.05 */
  float sinT = R / 3.2;
  float sr = (sinT / sqrt(1.0 - sinT * sinT)) / 1.05;
  float bgr = length(uv);
  vec2 p = uv / sr;
  float pd = length(p);
  vec2 n = p / max(pd, 1e-4);

  vec3 col = vec3(0.0);
  float alpha = 0.0;
  if(pd < 1.0 + 2.0 / (sr * iResolution.y)){
    /* The lens: the deeper toward the rim, the further inward the glass reads
       the ribbons, which stretches them into radial fibres near the edge. */
    float bend = pow(min(pd, 1.0), 2.6);
    vec2 q = p - n * 0.70 * bend;
    float split = 0.030 * bend;
    vec3 mid = lightAt(q, sr, bend);
    float midL = dot(mid, LUMA);
    /* The dispersion: where a read a little outward or inward outruns the
       middle one, the excess fringes in the palette's warm or cool tone. Split
       by channel instead, the lens would paint red and green the orb never
       wears. */
    vec3 warmT = mix(linear(uWork[3]), uTintCol, uTint);
    vec3 coolT = mix(linear(uWork[4]), uTintCol, uTint);
    warmT /= max(dot(warmT, LUMA), 1e-3);
    coolT /= max(dot(coolT, LUMA), 1e-3);
    vec3 acc = mid
      + 0.6 * warmT * max(dot(lightAt(q - n * split, sr, bend), LUMA) - midL, 0.0)
      + 0.6 * coolT * max(dot(lightAt(q + n * split, sr, bend), LUMA) - midL, 0.0);

    /* Working burns at the centre, large, white and steady. Ingest has a
       smaller, cooler heart that breathes in: it tightens as it brightens. The
       stopped states keep theirs dark. */
    float burn = smoothstep(0.4, 0.9, uLevel) * uForm * (1.0 - uIngest) * (1.0 - uTint);
    float heat = 1.1 * burn * (0.85 + 0.15 * sin(ta * 2.3));
    acc += mix(linear(uWork[1]), vec3(1.0), 0.5) * exp(-pow(pd / 0.24, 2.0)) * heat;
    float breathe = 0.5 + 0.5 * sin(ta * 1.6);
    float heartR = mix(0.17, 0.11, breathe);
    acc += mix(linear(uWork[2]), vec3(1.0), 0.3) * exp(-pow(pd / heartR, 2.0))
         * uIngest * (0.3 + 0.55 * breathe);

    float expo = 2.2 * (1.0 + 0.56 * uLevel);
    /* the toe keeps the gaps between sheets dark, the shoulder lets the
       folds run hot without clipping flat */
    vec3 body = pow(vec3(1.0) - exp(-acc * expo), vec3(1.45));

    /* The glass, laid over the light: a thin band of the refraction base on
       the rim, a cool and a warm dispersion lobe, a shade lower right, a key
       light upper left and a fill lower right. */
    float rimBand = 1.0 - smoothstep(0.0, 0.11, 1.0 - pd);
    float optic = pow(rimBand, 1.3);
    vec3 base = mix(linear(uWork[1]), uTintCol, uTint * 0.7);
    vec3 cool = mix(linear(uWork[4]), uTintCol, uTint * 0.7);
    vec3 warm = mix(linear(uWork[3]), uTintCol, uTint * 0.7);
    body = mix(body, base, optic * 0.06);
    body = mix(body, cool, optic * 0.23 * lobe(n, vec2(0.84, 0.54), -0.32, 1.8));
    body = mix(body, warm, optic * 0.23 * lobe(n, vec2(-0.62, -0.78), -0.28, 2.0));
    body *= 1.0 - optic * 0.042 * (0.15 + 0.85 * max(dot(n, vec2(0.45, -0.89)), 0.0));
    body = mix(body, warm, optic * 0.39 * lobe(n, vec2(-0.68, 0.73), 0.2, 2.8));
    body = mix(body, base, optic * 0.28 * lobe(n, vec2(0.74, -0.67), 0.4, 3.6));

    /* On paper the ball goes opaque and the ribbons glow inside it. The body
       takes the tokens as they are, not decoded to light: decoded, the deep
       stop reads near-black instead of the orb's own purple. */
    float ndv = sqrt(max(1.0 - pd * pd, 0.0));
    float grad = 0.5 + 0.5 * dot(n, vec2(-0.7071, 0.7071));
    vec3 ball = mix(uBody[0], uBody[1], grad);
    ball = mix(ball, uTintCol * 0.3, uTint * 0.85);
    /* a sphere still has to turn away from the light, or it reads as a disc */
    ball *= (0.68 + 0.42 * ndv) * (1.0 - 0.3 * smoothstep(0.55, 1.0, pd));

    float px = 1.5 / iResolution.y;
    float solid = 1.0 - smoothstep(sr - px, sr + px, bgr);
    /* quieter on dark: a dark page is already dark, and a full body there is a
       hole in it rather than a sphere on it */
    col   = mix(body + ball * DARKBODY, ball + body, uPaper);
    alpha = solid;
  }

  /* the stroke and the light the ball throws, its hue walking the palette */
  float aang = (atan(uv.y, uv.x) + PI) / TAU;
  vec3 rimCol = normalize(mix(mix(linear(uWork[1]), linear(uWork[4]), 0.5 + 0.5 * sin(TAU * aang + ta * 0.2)),
                              uTintCol, uTint) + 1e-4);
  float ring = exp(-pow((bgr - (sr + 0.0016)) / 0.0060, 2.0));
  float ringA = clamp(ring * (0.40 + 0.30 * uLevel), 0.0, 1.0);
  col   = mix(col, rimCol, ringA);
  alpha = max(alpha, ringA);

  /* On paper the same falloff is the shade the object casts, so it darkens. A
     bloom is light added to what is behind it, so a dark page gets less. */
  float outer = max(bgr - sr, 0.0);
  vec3 glowTint = mix(rimCol, linear(uWork[0]), 0.45);
  float glowA = (exp(-outer * 44.0) * 0.10 + exp(-outer * 14.0) * 0.018)
              * (0.60 + 0.40 * uLevel) * mix(DARKGLOW, 1.0, uPaper);
  float lit = 1.0 - uPaper;
  col   = mix(col, mix(glowTint, uBody[0], uPaper), (1.0 - alpha) * glowA);
  alpha = max(alpha, glowA * mix(1.0, 0.35, uPaper));
  col  *= 1.0 - 0.10 * lit * smoothstep(0.5, 1.5, bgr);

  col += (bayer(frag + 2.0) - 0.5) / 255.0;

  /* premultiplied, which is what the canvas compositor expects */
  vec3 srgb = pow(max(col, 0.0), vec3(1.0/2.2));
  outColor = vec4(srgb * alpha, alpha);
}`;

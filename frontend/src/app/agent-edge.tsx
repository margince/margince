// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useAgentEdge } from "./agent-edge-signal";
import "./agent-edge.css";

/**
 * The agent, in the margins of the workspace it works on.
 *
 * The old spelling of this was a lit ring travelling the agent's own card in the
 * sidebar, which had two problems. It decorated the agent's furniture rather than
 * the work, and it ran whether or not anything was happening, which is how a
 * signal becomes wallpaper.
 *
 * Here the product's own name is the design. Margince works in the margins: it
 * reads what is already on the screen, and it may not act without permission. So
 * the periphery of the whole window is where it shows, and it shows two separate
 * things:
 *
 * - **Reading.** The window's edge lights, and colour travels round it while work
 *   is in flight. It is the only thing here that moves, and it stops when the
 *   work does, so movement always means something is actually happening.
 * - **Waiting.** Something is staged and needs a person. The margin closes into a
 *   complete contour and holds perfectly still. Stillness is the signal: a thing
 *   that cannot proceed should not look busy. It dissolves when the queue does.
 *
 * At rest it draws nothing at all.
 *
 * Decorative throughout: every reading it carries is also written in words in the
 * rail, so this is `aria-hidden` and takes no pointer. A reader who cannot see it
 * loses nothing.
 */
export function AgentEdge() {
  const { reading, waiting } = useAgentEdge();
  return (
    <div
      className="agentedge"
      // Present or absent rather than "true"/"false": a CSS attribute selector
      // matches on presence, and `data-reading="false"` would light the margin.
      data-reading={reading ? "" : undefined}
      data-waiting={waiting ? "" : undefined}
      aria-hidden="true"
    >
      {reading && <LitEdge />}
      <span className="agentedge-wait" />
    </div>
  );
}

/**
 * The lit edge: drifting bodies of light, clipped to the window's rim.
 *
 * The light is BEHIND the edge and the edge is a window onto it. What changed
 * here, after several attempts that read as mechanical, is what the light is made
 * of and how it moves.
 *
 * The whole rim is lit at once, by a gradient that does not move, so no side of
 * the window is ever waiting its turn. One soft arc travels over the top of that,
 * which is a swell passing through a lit edge rather than a light touring a dark
 * one: the difference is whether the frame is dark where the arc is not.
 *
 * On top of that even light, five bodies vary it IN PLACE. Each drifts a few
 * percent of the window, no further, and swells on its own period; none of the
 * periods are related. Where two of them overlap the rim brightens, as they part it
 * dims, and because none of them goes anywhere the undulation happens everywhere
 * at once rather than sweeping past.
 *
 * The rim itself is warped as well, by the turbulence filter below, and that is
 * the other half of it: the bodies make the LIGHT undulate, the displacement makes
 * the EDGE undulate. Neither alone billows.
 *
 * The filter sits on the wrapper rather than on the rims. A filter applies to an
 * element before its own mask clips it, so displacing a rim directly warps the
 * light inside a dead-straight band; displacing the wrapper warps what the
 * children already cut.
 */
function LitEdge() {
  return (
    <div className="agentedge-lit">
      {/* Deepest first: the spill, which is the light the frame throws onto the
          page. It is the one layer not clipped to a rim, and it is what makes the
          rim look like a light rather than a coloured line. */}
      <span className="agentedge-spill">
        <Field />
      </span>
      {/* Then the near glow, then the crisp rim on top of its own light. Each
          carries its own copy of the field: one field clipped at three widths
          would be three rims of the same brightness rather than a falloff. */}
      <span className="agentedge-bloom">
        <Field />
      </span>
      <span className="agentedge-ring">
        <Field />
      </span>
      {/* The pulse: one soft arc that DOES travel the perimeter, over the top of
          the light that does not. On its own a travelling arc leaves the rest of
          the frame dark until it comes round; over an already-lit rim it reads as
          a swell passing through, which is the one kind of going-around motion
          worth having here. */}
      <span className="agentedge-pulse">
        <span className="agentedge-sweep" />
      </span>
      <WabernFilter />
    </div>
  );
}

/**
 * Five bodies of light, sized and coloured in the stylesheet.
 *
 * Five and not three: with too few, the gaps between them are long enough that a
 * stretch of rim goes dark for seconds at a time, which reads as flickering rather
 * than as billowing.
 */
function Field() {
  return (
    <span className="agentedge-field">
      <span className="agentedge-body agentedge-body-1" />
      <span className="agentedge-body agentedge-body-2" />
      <span className="agentedge-body agentedge-body-3" />
      <span className="agentedge-body agentedge-body-4" />
      <span className="agentedge-body agentedge-body-5" />
    </span>
  );
}

/**
 * The warp, and the reason the edge undulates rather than merely brightening.
 *
 * `feTurbulence` builds a smooth noise field and `feDisplacementMap` pushes every
 * pixel it is handed by what it finds there. What it is handed is the finished
 * rim, so the rim thickens in one stretch and thins in the next.
 *
 * The field itself drifts, slowly, on a long period. Displacing by a STILL field
 * gives a fixed set of curves for the light to slide through, which reads as
 * frosted glass rather than as anything moving.
 *
 * `scale` is capped by the bleed in agent-edge.css: displace further than the band
 * hangs past the viewport and the warp pulls the rim off the screen edge, which
 * shows up as gaps along a side.
 */
function WabernFilter() {
  return (
    <svg className="agentedge-defs" aria-hidden="true">
      <filter
        id="agentedgewabern"
        // Room to displace into: at the default filter region the outermost
        // pixels have nowhere to move and the crests flatten against its edge.
        x="-10%"
        y="-10%"
        width="120%"
        height="120%"
        colorInterpolationFilters="sRGB"
      >
        <feTurbulence
          type="fractalNoise"
          baseFrequency="0.0035 0.011"
          numOctaves={2}
          seed={9}
          result="field"
        >
          <animate
            attributeName="baseFrequency"
            dur="19s"
            values="0.0035 0.011;0.006 0.016;0.003 0.008;0.0035 0.011"
            repeatCount="indefinite"
          />
        </feTurbulence>
        <feDisplacementMap
          in="SourceGraphic"
          in2="field"
          scale={9}
          xChannelSelector="R"
          yChannelSelector="G"
        />
      </filter>
    </svg>
  );
}

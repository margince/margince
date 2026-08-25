// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { usePrefersReducedMotion } from "../design-system/motion";
import { type EdgeHues, readHue } from "./agent-edge-gl";
import { runEdgeLoop } from "./agent-edge-loop";
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
 * - **Reading.** Waves travel the window's rim while work is in flight, with the
 *   colour drifting inside them. It is the only thing here that moves, and it
 *   stops when the work does, so movement always means something is happening.
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
      // matches on presence, and `data-waiting="false"` would draw the contour.
      data-waiting={waiting ? "" : undefined}
      aria-hidden="true"
    >
      {reading && <LitEdge />}
      <span className="agentedge-wait" />
    </div>
  );
}

/**
 * The lit edge.
 *
 * A shader, and the third technique this surface has worn. The two before it were
 * a masked band carrying moving gradients, then that band warped by an SVG
 * turbulence filter. Both failed the same two tests, for reasons no amount of
 * tuning could reach:
 *
 * - **The crests could not travel.** A gradient inside a band slides across the
 *   window rather than along the edge, and a turbulence field cannot be slid along
 *   a perimeter without popping when its loop comes round. Here the wave's phase
 *   is a function of distance ALONG the rim, so travelling is what it does.
 * - **It could not be smooth AND cheap.** Displacement moves whole pixels, so the
 *   rim came out notched, and hiding that took blurs on both sides of the
 *   displacement. Together with fifteen animated blurred elements that was enough
 *   full-viewport filtering to make a fullscreen window stutter. The rim's edge is
 *   now one `smoothstep` per fragment: smooth at any size, and one draw call.
 *
 * Everything about WHEN it draws lives in `agent-edge-loop.ts`, which is where a
 * test can reach it. What stays here is the mounting: read the hues off the
 * document, start the loop, and wear the static rim if the host cannot run it.
 */
function LitEdge() {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [live, setLive] = useState(false);
  const reduced = usePrefersReducedMotion();

  useEffect(() => {
    const element = canvas.current;
    if (!element) {
      return;
    }
    // Read once per mount rather than per frame: a theme change remounts this
    // (the signal clears on unmount), and `getComputedStyle` is a layout read
    // with no business in a draw loop.
    const hues: EdgeHues = [
      readHue("--teal"),
      readHue("--orbJade"),
      readHue("--orbMint"),
      readHue("--orbLime"),
      readHue("--orbAmber"),
    ];
    const loop = runEdgeLoop(element, hues, {
      reduced,
      // A context lost to a GPU reset is not recoverable in place: the program
      // and the buffer went with it, so the honest picture of what this machine
      // can draw is the static rim.
      onLost: () => setLive(false),
    });
    setLive(loop !== null);
    return () => loop?.stop();
  }, [reduced]);

  return (
    <canvas
      ref={canvas}
      // The static rim is what a host without WebGL2 wears. It carries no waves,
      // which is honest: the fallback says the agent is working without claiming
      // to show how.
      className={live ? "agentedge-canvas" : "agentedge-canvas agentedge-still"}
    />
  );
}

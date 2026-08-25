// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { usePrefersReducedMotion } from "../design-system/motion";
import { createEdgeRenderer, type EdgeHues, readHue } from "./agent-edge-gl";
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

/** Thirty a second. The waves are slow and the shader is the most expensive thing
 *  on the surface; at a display's own rate this is the same picture drawn twice. */
const FRAME_MS = 1000 / 30;

/** How long the light takes to arrive and to leave, in seconds. Nothing here
 *  appears or cuts: an edge that snapped on would read as a fault. */
const FADE = 0.45;

/**
 * The lit edge.
 *
 * A shader, and the third technique this surface has worn. The two before it were
 * a masked band carrying moving gradients, then that band warped by an SVG
 * turbulence filter. Both failed the same two tests, and for reasons no amount of
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
 * The whole thing is unmounted when the agent is not reading, so a quiet screen
 * pays nothing at all.
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
    // The hues are read once per mount rather than per frame: a theme change
    // remounts this (the signal clears on unmount), and `getComputedStyle` is a
    // layout read that has no business in a draw loop.
    const hues: EdgeHues = [
      readHue("--teal"),
      readHue("--orbJade"),
      readHue("--orbMint"),
      readHue("--orbLime"),
      readHue("--orbAmber"),
    ];
    const renderer = createEdgeRenderer(element, hues);
    setLive(renderer !== null);
    if (!renderer) {
      return;
    }

    let handle = 0;
    let drawnAt = 0;
    let born = 0;
    let stopped = false;

    const measure = () => {
      renderer.resize(
        window.innerWidth,
        window.innerHeight,
        window.devicePixelRatio || 1,
      );
    };

    const tick = (now: number) => {
      handle = requestAnimationFrame(tick);
      if (born === 0) {
        born = now;
      }
      if (now - drawnAt < FRAME_MS) {
        return;
      }
      drawnAt = now;
      const age = (now - born) / 1000;
      // Reduced motion keeps the light and gives up the travel: the waves hold
      // the shape they arrived in, which still says the agent is working.
      renderer.draw({
        time: reduced ? 0 : age,
        level: Math.min(age / FADE, 1),
      });
    };

    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(document.documentElement);
    handle = requestAnimationFrame(tick);

    // A context lost to a GPU reset is not recoverable in place: the program and
    // the buffer are gone with it. Stopping AND reporting, because reporting
    // alone would leave the loop drawing into a context that no longer exists.
    const onLost = (event: Event) => {
      event.preventDefault();
      stopped = true;
      cancelAnimationFrame(handle);
      setLive(false);
    };
    element.addEventListener("webglcontextlost", onLost);

    return () => {
      element.removeEventListener("webglcontextlost", onLost);
      observer.disconnect();
      if (!stopped) {
        cancelAnimationFrame(handle);
      }
      renderer.dispose();
    };
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

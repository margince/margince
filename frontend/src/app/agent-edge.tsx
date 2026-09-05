// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { readHue } from "../design-system/margince-core-gl";
import { usePrefersReducedMotion } from "../design-system/motion";
import type { EdgeHues } from "./agent-edge-gl";
import { type EdgeLoop, runEdgeLoop } from "./agent-edge-loop";
import { type AgentEdgeRegister, useAgentEdge } from "./agent-edge-signal";
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
 * the periphery of the whole window is where it shows, and it shows ONE thing:
 * work in flight. Waves travel the window's rim while the agent is reading or
 * acting, with the colour drifting inside them, and they stop when the work
 * does — so movement here always means something is happening.
 *
 * In two registers, because two kinds of work light it and they are not read
 * the same way. The agent's own — a run, a call — is seconds to a minute, and
 * the rim is a little thicker and livelier for it. A mailbox import is minutes
 * to hours, and the same rim over that span was a lamp left on in the corner of
 * every screen; it takes a thinner, calmer rim that still travels, so mail
 * arriving is still visibly happening without being the loudest thing on the
 * page all afternoon. Which register is the rail's call, published with the
 * reading (agent-edge-signal.ts).
 *
 * At rest it draws nothing at all.
 *
 * A STAGED DECISION IS NOT DRAWN HERE. It used to close the margin into a
 * complete, still contour, and on any installation with an unanswered queue that
 * is a green ring around the whole window for as long as the queue stands —
 * which is a signal becoming wallpaper, the exact failure this surface replaced.
 * The rail says it in words instead ("2 waiting for you"), where it can carry
 * the COUNT, which a contour never could.
 *
 * Decorative throughout: every reading it carries is also written in words in the
 * rail, so this is `aria-hidden` and takes no pointer. A reader who cannot see it
 * loses nothing.
 */
export function AgentEdge() {
  const { reading, register } = useAgentEdge();
  // The edge outlives the reading that lit it, by exactly as long as it takes to
  // go out. Unmounting on `reading` alone cut the light dead the instant the work
  // finished, and a light that vanishes reads as something breaking; the shader
  // owns the dimming (its own level uniform), so what this holds is only the
  // canvas it dims on.
  const [lingering, setLingering] = useState(false);
  useEffect(() => {
    if (reading) {
      setLingering(true);
    }
  }, [reading]);
  return (
    <div className="agentedge" aria-hidden="true">
      {(reading || lingering) && (
        <LitEdge
          lit={reading}
          register={register}
          onDark={() => setLingering(false)}
        />
      )}
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
function LitEdge({
  lit,
  register,
  onDark,
}: {
  lit: boolean;
  register: AgentEdgeRegister;
  onDark: () => void;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const loop = useRef<EdgeLoop | null>(null);
  const [live, setLive] = useState(false);
  const reduced = usePrefersReducedMotion();
  // Read through a ref so a caller's new closure does not restart the loop:
  // rebuilding the GL program on every render of the parent would throw away the
  // light this component exists to keep. Declared here because the effect below
  // reads it, and a const is not hoisted.
  const onDarkRef = useRef(onDark);
  onDarkRef.current = onDark;
  // `lit` is read by the effect that BUILDS the loop, and that effect must not
  // rebuild when `lit` changes: rebuilding the GL program is the one thing this
  // component exists to avoid. A ref is how the builder sees the current value
  // without taking it as a dependency.
  const litRef = useRef(lit);
  litRef.current = lit;
  const registerRef = useRef(register);
  registerRef.current = register;

  useEffect(() => {
    const element = canvas.current;
    if (!element) {
      return;
    }
    // Read once per mount rather than per frame: a theme change remounts this
    // (the signal clears on unmount), and `getComputedStyle` is a layout read
    // with no business in a draw loop.
    // The AI hue and the orb's own working tones, so the lit window and the orb
    // inside it are one object rather than two decorations. --teal used to open
    // this run and cannot any more: trust.css spends it on HUMAN provenance, and
    // an edge that says a machine is working has no business starting there.
    // Amber stays as the one warm stop, for the same reason the orb keeps it.
    // Literal tokens only, no --orbBody: that one is declared as var(--ai) and a
    // hue this seam cannot parse draws mid-grey rather than failing.
    const hues: EdgeHues = [
      readHue("--ai"),
      readHue("--orbMid"),
      readHue("--orbGlow"),
      readHue("--orbBright"),
      readHue("--orbAmber"),
    ];
    const started = runEdgeLoop(element, hues, {
      reduced,
      // A context lost to a GPU reset is not recoverable in place: the program
      // and the buffer went with it, so the honest picture of what this machine
      // can draw is the static rim.
      onLost: () => {
        // The handle goes too. A stopped loop still referenced here would be
        // asked to setLit(false) later, would do nothing, and would never report
        // going dark, which leaves the canvas mounted for the life of the page.
        loop.current = null;
        setLive(false);
      },
      onDark: () => onDarkRef.current(),
    });
    loop.current = started;
    // The register before the light: a fresh loop takes its register at once
    // while dark, so the first lit frame is already in the right one.
    started?.setRegister(registerRef.current);
    // A fresh loop opens with its light asked for. When this rebuild happened
    // while the edge was already on its way out (a motion-preference change
    // mid-fade), the replacement relit and stayed lit, because the effect below
    // only runs when `lit` CHANGES and it had not.
    started?.setLit(litRef.current);
    setLive(started !== null);
    return () => {
      started?.stop();
      loop.current = null;
    };
  }, [reduced]);

  // Only while the light is asked for. The signal wears the agent's register
  // the moment the work stops, whatever it was lit in, and taking that word
  // while the light is going out would thicken a rim on its way to nothing.
  useEffect(() => {
    if (lit && live) {
      loop.current?.setRegister(register);
    }
  }, [register, lit, live]);

  // `live` is a dependency, not decoration: losing the context clears the loop
  // while `lit` may already be false, and without re-running here nothing would
  // ever report the edge dark and the static rim would outlive the work.
  useEffect(() => {
    const running = loop.current;
    // Gated on `live` as well as the handle, which is what makes the dependency
    // real rather than a nudge: `live` IS the answer to "is there a shader to
    // dim", and it turns false the moment the context is lost.
    if (live && running) {
      running.setLit(lit);
      return;
    }
    // No shader to dim, either because this host has none or because the context
    // was just lost. Holding the rim would leave it on screen after the work
    // stopped, so report at once and let the caller unmount.
    if (!lit) {
      onDarkRef.current();
    }
  }, [lit, live]);

  return (
    <canvas
      ref={canvas}
      // The static rim is what a host without WebGL2 wears. It carries no waves,
      // which is honest: the fallback says the agent is working without claiming
      // to show how.
      className={live ? "agentedge-canvas" : "agentedge-canvas agentedge-still"}
      // For the static rim, which has no loop to ease and wears its register
      // from here; and it is the one place a test can read which register the
      // edge was told, since the shader's frames never reach the DOM.
      data-register={register}
    />
  );
}

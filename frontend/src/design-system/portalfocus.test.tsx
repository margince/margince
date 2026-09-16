// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { EvidenceMark } from "./evidencemark";
import { Popover } from "./popover";

// A PORTALLED PANEL BEHAVES, FOR FOCUS, AS THOUGH IT SAT BY ITS TRIGGER.
//
// Both components that portal a panel are asked the same three questions here,
// because the rule they share is one rule: two copies of it drifted into having
// the same two holes, which is what put this suite in front of the hook rather
// than in front of either caller.
//
// Each case is written against what a reader would DO — press, tab, walk the
// pointer away — rather than against the hook's own shape, so a third component
// adopting the rule joins by being added to the table below.
//
// The one case NOT here is a panel opened inside a dialog, where this rule
// stands down and the dialog's trap owns Tab. That belongs to the trap and is
// held from its side, in modal.test.tsx — "holds Tab in the open panel the
// reader is in" fails the moment this rule stops standing down.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// THE PAGE AROUND THE TRIGGER, which is the whole subject: Tab out of the panel
// has to reach "after", the control that follows the trigger on screen, and not
// whatever the body happens to render next.
function page(trigger: ReactNode) {
  return (
    <LocaleProvider initial="en">
      <button type="button">before</button>
      {trigger}
      <button type="button">after</button>
    </LocaleProvider>
  );
}

// Longer than any window the hover hook measures — its settle ceiling, its
// close grace, its poll. Advancing by it settles whatever the pointer is doing
// without this file holding a second copy of numbers that belong to that hook.
const PAST_EVERY_THRESHOLD = 1000;

const PANELS = [
  {
    name: "Popover",
    render: () =>
      render(
        page(
          <Popover label="How it stands" onHover>
            <button type="button">Full history</button>
          </Popover>,
        ),
      ),
    triggerName: "How it stands",
  },
  {
    name: "EvidenceMark",
    render: () =>
      render(
        page(
          <EvidenceMark
            value="Google"
            source={{ provenance: { kind: "agent", agent: "capture" } }}
            onOpenHistory={() => {}}
            historyLabel="Full history"
          />,
        ),
      ),
    triggerName: /Google/,
  },
];

describe.each(PANELS)(
  "$name's portalled panel",
  ({ render: mount, triggerName }) => {
    // A press is the reader asking for the panel, so focus follows them into it.
    // Everything below stands on this: a panel nobody's focus ever reaches has no
    // focus to lose or to hand back.
    it("takes focus into its own control when a press opens it", async () => {
      mount();

      await userEvent.click(screen.getByRole("button", { name: triggerName }));

      await waitFor(() =>
        expect(document.activeElement?.textContent).toBe("Full history"),
      );
    });

    // Tab off the last stop in the panel goes where it would have gone if the
    // panel were the trigger's next sibling — which is what the portal took away.
    // Before this it walked into whatever the body rendered next: a toast region,
    // the next portal, the top of the document.
    it("tabs forward to the control after the trigger, not after the body", async () => {
      const user = userEvent.setup();
      mount();
      await user.click(screen.getByRole("button", { name: triggerName }));
      await waitFor(() =>
        expect(document.activeElement?.textContent).toBe("Full history"),
      );

      await user.tab();

      expect(document.activeElement?.textContent).toBe("after");
    });

    // And backward off the first stop returns to the trigger, so a reader who
    // over-shot can step back into the value the panel is about.
    it("tabs backward to the trigger", async () => {
      const user = userEvent.setup();
      mount();
      await user.click(screen.getByRole("button", { name: triggerName }));
      await waitFor(() =>
        expect(document.activeElement?.textContent).toBe("Full history"),
      );

      await user.tab({ shift: true });

      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: triggerName }),
      );
    });

    // THE CLOSE THAT DROPPED FOCUS ON THE FLOOR. A pointer leaving the trigger
    // schedules a close through the hover grace period, and the panel then
    // unmounts with one of its controls still focused — so focus fell to <body>
    // and the reader's next Tab started the page again from the top.
    //
    // ON A CLOCK THIS TEST OWNS, `performance` included, for the reason
    // hoverintent.ts states in its own header: the hook reasons in real
    // milliseconds, so a case that renders one and then asserts is racing a timer
    // and loses on whichever machine is busiest. `PAST_EVERY_THRESHOLD` is
    // deliberately a round number well beyond the hook's ceiling and grace rather
    // than a copy of either — the claim here is about what happens after they
    // elapse, not about what they are.
    it("hands focus back to the trigger when a passing pointer closes it", () => {
      vi.useFakeTimers({
        toFake: [
          "setTimeout",
          "clearTimeout",
          "setInterval",
          "clearInterval",
          "performance",
        ],
      });
      mount();
      const trigger = screen.getByRole("button", { name: triggerName });
      act(() => {
        fireEvent.click(trigger);
      });
      expect(document.activeElement?.textContent).toBe("Full history");

      // The pointer settles on the trigger, which is what arms the grace-period
      // close: the hook schedules one only for a pointer it saw arrive.
      act(() => {
        fireEvent.pointerEnter(trigger);
      });
      act(() => {
        vi.advanceTimersByTime(PAST_EVERY_THRESHOLD);
      });
      act(() => {
        fireEvent.pointerLeave(trigger);
      });
      act(() => {
        vi.advanceTimersByTime(PAST_EVERY_THRESHOLD);
      });

      expect(screen.queryByRole("button", { name: "Full history" })).toBeNull();
      expect(document.activeElement).toBe(trigger);
    });
  },
);

// AND THE RETURN DOES NOT OVER-FIRE.
//
// A control inside the panel may close it and send the reader somewhere on
// purpose — the receipt's "Full history" opens a drawer and focuses it. A
// return firing there would drag them straight back out of what they just
// opened, so the rule answers only the case where the unmount left focus on
// `<body>` with nobody claiming it.
//
// Asked of the evidence mark alone, because it is the one of the two whose
// panel carries a control that closes the panel itself.
it("leaves focus where a closing control put it", async () => {
  const user = userEvent.setup();
  render(
    <LocaleProvider initial="en">
      <button type="button" id="drawer">
        the drawer this opened
      </button>
      <EvidenceMark
        value="Google"
        source={{ provenance: { kind: "agent", agent: "capture" } }}
        onOpenHistory={() => {
          document.getElementById("drawer")?.focus();
        }}
        historyLabel="Full history"
      />
    </LocaleProvider>,
  );
  const trigger = screen.getByRole("button", { name: /Google/ });
  await user.click(trigger);
  await waitFor(() =>
    expect(document.activeElement?.textContent).toBe("Full history"),
  );

  await user.click(screen.getByRole("button", { name: "Full history" }));

  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Full history" })).toBeNull(),
  );
  expect(document.activeElement?.textContent).toBe("the drawer this opened");
});

import {
  type CSSProperties,
  type RefObject,
  useLayoutEffect,
  useState,
} from "react";

/** The air between the rail and the panel it opens. */
const PANEL_GAP = 8;

/**
 * How wide the panel is beside the rail, and the least air it leaves.
 *
 * Wide enough that a count tile holds its label on one line. If "Duplicate
 * pairs open" wraps under its own number, a figure with a name becomes two
 * stacked fragments.
 */
const PANEL_WIDTH = 408;

const PANEL_MARGIN = 12;

export type PanelFrame = Readonly<{
  left: number;
  right?: number;
  bottom: number;
  width?: number;
  maxHeight: number;
  /**
   * Where the notch under the panel points, in viewport coordinates.
   *
   * Only the phone frame carries one: there the panel stands over the anchor
   * rather than beside it. A full-width sheet with nothing pointing at what
   * opened it could have come from anywhere. It is measured from the anchor,
   * not assumed to be the middle of the screen. So the notch still lands on the
   * orb if the bar's cells ever stop being symmetric.
   */
  caret?: number;
}>;

/**
 * The two custom properties the portalled wrapper is placed by.
 *
 * Declared rather than asserted onto `CSSProperties`. React's own type carries
 * the CSS properties it knows, and a cast would say these two are among them.
 * The intersection says what is true: the wrapper takes a style object that is
 * one of those plus the two this file mints.
 */
type NotchPlacement = CSSProperties &
  Readonly<{
    "--arCaretX"?: string;
    "--arPanelBottom": string;
  }>;

/**
 * The notch's place, handed to the stylesheet rather than drawn here.
 *
 * Where it points is a measurement, and how it is drawn is the sheet's
 * business. So the frame travels as custom properties on the portalled wrapper.
 * Beside a sidebar there is no notch and no `--arCaretX`. The panel and the
 * card that opened it already touch, and a tail would have no gap to cross.
 */
export function looseStyle(frame: PanelFrame): NotchPlacement {
  return {
    "--arCaretX": frame.caret === undefined ? undefined : `${frame.caret}px`,
    "--arPanelBottom": `${frame.bottom}px`,
  };
}

/**
 * The frame over the phone bar: the bar's own span, clear of the well.
 *
 * Edge to edge with the bar rather than inset by a margin of its own. The panel
 * hangs off one of the bar's cells, so the two are one object at two distances.
 * A panel inset by a different amount reads as a sheet that happened to arrive
 * over the bar. It falls back to its own margin only where no bar was handed
 * in, which is a caller that has none.
 */
function overTheBar(well: DOMRect, bar: DOMRect | undefined): PanelFrame {
  return {
    left: bar ? bar.left : PANEL_MARGIN,
    right: bar ? globalThis.innerWidth - bar.right : PANEL_MARGIN,
    bottom: globalThis.innerHeight - well.top + PANEL_GAP,
    maxHeight: well.top - PANEL_GAP * 2,
    caret: well.left + well.width / 2,
  };
}

/**
 * The frame beside the sidebar: bottom-aligned to the card, so opening the panel
 * does not move the thing that opened it.
 */
function besideTheCard(card: DOMRect): PanelFrame {
  const height = globalThis.innerHeight;
  return {
    left: card.right + PANEL_GAP,
    bottom: Math.max(PANEL_MARGIN, height - card.bottom),
    width: PANEL_WIDTH,
    maxHeight: height - PANEL_MARGIN * 2,
  };
}

/**
 * Where the panel goes, in viewport coordinates.
 *
 * It is fixed and portalled to the body, not positioned inside its block. The
 * rail scrolls its destinations, so it carries `overflow-x: hidden`. Anything
 * absolutely positioned beside the rail is clipped by it.
 *
 * Beside the rail on a desktop, bottom-aligned to the block, so opening it does
 * not move the thing that opened it. On a phone the rail is the bottom bar, and
 * there is no beside. The panel takes the width of the screen and sits above
 * the block instead.
 */
export function usePanelFrame(
  card: RefObject<HTMLElement | null>,
  well: RefObject<HTMLElement | null>,
  bar: RefObject<HTMLElement | null> | undefined,
  open: boolean,
  phone: boolean,
): PanelFrame | null {
  const [frame, setFrame] = useState<PanelFrame | null>(null);
  // Before paint, so the panel is never shown at the wrong place for one frame.
  useLayoutEffect(() => {
    if (!open) {
      setFrame(null);
      return;
    }
    const place = () => {
      // Two anchors, because the panel stands in two places. On the sidebar it
      // sits beside the whole card, which is what the reader is looking at. On
      // the phone bar it sits above the round well, which rises out of the
      // bar's top edge. A frame measured from the cell behind the well would
      // open the panel across the orb it belongs to.
      const box = (phone ? well : card).current?.getBoundingClientRect();
      if (!box) {
        return;
      }
      setFrame(
        phone
          ? overTheBar(box, bar?.current?.getBoundingClientRect())
          : besideTheCard(box),
      );
    };
    place();
    // The anchor moves when the rail collapses, when the window changes size and
    // when anything it sits in scrolls. So the frame follows all three, rather
    // than being measured once at open.
    globalThis.addEventListener("resize", place);
    globalThis.addEventListener("scroll", place, true);
    return () => {
      globalThis.removeEventListener("resize", place);
      globalThis.removeEventListener("scroll", place, true);
    };
  }, [bar, card, well, open, phone]);
  return frame;
}

// How big a view is, told to the host that frames it.

type Size = { width: number; height: number };
type Send = (message: Record<string, unknown>) => void;

/**
 * followContentSize tells the host how big this document's content is, now and
 * whenever it changes, as `ui/notifications/size-changed` (SEP-1865). A host
 * draws a view in a frame of its own default height until it is told
 * otherwise, so a view that never says cuts its own panel off part way down —
 * the reader sees the head and a hairline and none of the rows.
 *
 * `send` is the bridge's own, so the report goes to the host's pinned origin
 * like every other message after the handshake. The bridge starts this once
 * the first result is drawn, so the first report is the answer's height.
 */
export function followContentSize(send: Send): void {
  let reported: Size | null = null;
  let pending = false;
  const report = () => {
    pending = false;
    const size = measure();
    if (reported?.width === size.width && reported.height === size.height) {
      return;
    }
    reported = size;
    send({ method: "ui/notifications/size-changed", params: size });
  };
  // One measurement per frame, however many resizes landed in it.
  const schedule = () => {
    if (pending) return;
    pending = true;
    requestAnimationFrame(report);
  };
  const observer = new ResizeObserver(schedule);
  observer.observe(document.documentElement);
  observer.observe(document.body);
  schedule();
}

/**
 * measure reads the content's size the way the extension's own SDK reads it.
 *
 * The height is read with the root at `max-content`, not as it stands: the root
 * stretches to the frame, so reading it as it stands reports the frame's height
 * back to the host and a frame shorter than its content never grows. The width
 * is the frame's own, because the host decides it and the content wraps to it.
 */
function measure(): Size {
  const root = document.documentElement;
  const declared = root.style.height;
  root.style.height = "max-content";
  const height = Math.ceil(root.getBoundingClientRect().height);
  root.style.height = declared;
  return { width: Math.ceil(window.innerWidth), height };
}

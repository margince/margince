import { type ReactNode, type RefObject, useEffect, useRef } from "react";
import { Card } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Logomark } from "../../design-system/logomark";
import { usePrefersReducedMotion } from "../../design-system/motion";
import { useT } from "../../i18n";
import { usePublicLocale } from "./publiclocale";
import "../settings/preferences.css";

/** The frame of every page a message links to: one centred column, ground and
 * sign-off, speaking the language the link names. */
export function PublicPage({ children }: Readonly<{ children: ReactNode }>) {
  usePublicLocale();
  const column = useRef<HTMLDivElement>(null);
  useGlide(column);
  return (
    <div className="pref-page">
      <div className="pref-center arrive-stack" ref={column}>
        {children}
        <PublicFooter />
      </div>
    </div>
  );
}

/** The page every dead public link lands on: the answer as the heading, the
 * reason under it. The tab title names the answer, not the product. */
export function DeadLinkPage({ children }: Readonly<{ children: ReactNode }>) {
  const t = useT();
  const title = t("prefs.unsub.deadLinkTitle");
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
  return (
    <PublicPage>
      <Heading size="xlarge" className="t-display">
        {t("prefs.unsub.deadLinkTitle")}
      </Heading>
      <Card>
        <p>{children}</p>
      </Card>
    </PublicPage>
  );
}

const GLIDING = "data-gliding";

/** Slides the centred column to where a change of its height put it. A press
 * that grows the page then moves the control smoothly instead of in one jump. */
function useGlide(column: RefObject<HTMLElement | null>) {
  const reduced = usePrefersReducedMotion();
  useEffect(() => {
    const element = column.current;
    const page = element?.parentElement;
    if (!element || !page || reduced) {
      return;
    }
    // A page settling after its load is not something the reader did.
    let armed = false;
    const arm = () => {
      armed = true;
    };
    // offsetTop ignores transforms, so `top` is the layout position, not the glide's.
    let top: number | null = element.offsetTop;
    let pageWidth = page.offsetWidth;
    let frame = 0;
    const stop = () => {
      cancelAnimationFrame(frame);
      element.removeAttribute(GLIDING);
      element.style.transform = "";
    };
    const observer = new ResizeObserver((entries) => {
      if (element.offsetParent === null) {
        stop();
        top = null;
        return;
      }
      const moved = top === null ? 0 : top - element.offsetTop;
      top = element.offsetTop;
      // A window that changed width reflows the column; that is no press to answer.
      const reflowed = page.offsetWidth !== pageWidth;
      pageWidth = page.offsetWidth;
      const resized = entries.some((entry) => entry.target === element);
      if (!armed || moved === 0 || !resized || reflowed) {
        return;
      }
      const from = moved + glideOffset(element);
      stop();
      element.style.transform = `translateY(${from}px)`;
      frame = requestAnimationFrame(() => {
        element.setAttribute(GLIDING, "");
        element.style.transform = "";
      });
    });
    const settle = (event: TransitionEvent) => {
      if (event.target === element && event.propertyName === "transform") {
        element.removeAttribute(GLIDING);
      }
    };
    observer.observe(element);
    observer.observe(page);
    element.addEventListener("pointerdown", arm);
    element.addEventListener("keydown", arm);
    element.addEventListener("transitionend", settle);
    element.addEventListener("transitioncancel", settle);
    return () => {
      observer.disconnect();
      stop();
      element.removeEventListener("pointerdown", arm);
      element.removeEventListener("keydown", arm);
      element.removeEventListener("transitionend", settle);
      element.removeEventListener("transitioncancel", settle);
    };
  }, [column, reduced]);
}

/** Where a glide still under way has the column, so a second change starts from there. */
function glideOffset(element: HTMLElement): number {
  const matrix = /^matrix\((.+)\)$/.exec(getComputedStyle(element).transform);
  const offset = Number(matrix?.[1]?.split(",")[5]);
  return Number.isFinite(offset) ? offset : 0;
}

/**
 * Who sent this.
 *
 * A page that asks somebody to make a decision about their own mail owes
 * them the name of the product asking — otherwise an unbranded page
 * arriving from a link is indistinguishable from a phishing attempt.
 */
function PublicFooter() {
  const t = useT();
  return (
    <p className="public-foot">
      <Logomark size={15} />
      {t("prefs.sentVia")}
    </p>
  );
}

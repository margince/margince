import type { ReactNode } from "react";
import "./savebar.css";

// The unsaved draft of a whole settings page, pinned to the foot of the view so
// the one Save for several sections is reachable from whichever section the
// reader edited last.
export function SaveBar({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <section className="save-bar" aria-label={label}>
      {children}
    </section>
  );
}

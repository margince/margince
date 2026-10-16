import { type ReactNode, useRef } from "react";
import { type ScrollRegionName, useScrollRegion } from "./scrollregion";

/**
 * The box a table too wide for its column scrolls sideways in. It is the one
 * spelling of `.table-scroll`, so the overflow is a property of the table
 * rather than a knob each page answers for.
 *
 * Reachability is `useScrollRegion`'s (scrollregion.ts): the tab stop and the
 * name arrive only while the box holds something past its right edge.
 *
 * `label` names the region: a phrase, or `{ labelledBy }` for a heading already
 * on the page. It has no default, since "region" names no table.
 */
export function TableScroll({
  label,
  bleed,
  stickyFirst,
  className,
  children,
}: Readonly<{
  label: ScrollRegionName;
  bleed?: boolean;
  stickyFirst?: boolean;
  className?: string;
  children: ReactNode;
}>) {
  const box = useRef<HTMLDivElement | null>(null);
  const region = useScrollRegion(box, label);
  const bleedClass = bleed ? "table-scroll-bleed" : "";
  const stickyClass = stickyFirst ? "table-scroll-sticky" : "";
  const boxClasses = ["table-scroll", bleedClass, stickyClass, className ?? ""];
  return (
    <div ref={box} className={boxClasses.filter(Boolean).join(" ")} {...region}>
      {children}
    </div>
  );
}

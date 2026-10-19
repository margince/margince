// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EmptyState, Skeleton } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { RingDot, Rings } from "../../design-system/rings";
import { useT } from "../../i18n";
import { CallCard, TodayPanel } from "../../screens/record360";
import "./company360.css";
import { HEALTH_RANK } from "./companylookups";
import {
  type TodayDimension,
  type TodayReading,
  type TodayReadingInputs,
  useTodayReading,
} from "./companytoday.reading";

export {
  type TodayDimension,
  type TodayReading,
  useTodayReading,
} from "./companytoday.reading";

export type Company360 = components["schemas"]["Company360"];

/**
 * One rated dimension, and what stands behind it.
 *
 * The chip is a TRIGGER, because three words cannot carry a rating a reader
 * can check: "Commercial · Good" says neither what was weighed nor what it was
 * read from, so a reader either takes it on trust or goes looking for the
 * working somewhere else on the page. Resting on the chip answers both, in
 * that order — what the dimension is, then the reading the rating was made
 * from.
 *
 * `onHover` and not a click alone: this is an aside taken in on the way past,
 * where a click is a step a reader should not have to take to check a claim.
 * The click stays for the touch screen and the keyboard, which have no hover.
 */
function DimensionChip({ dimension }: Readonly<{ dimension: TodayDimension }>) {
  const t = useT();
  return (
    <div className="co-360-reading">
      <dt className="t-caption">
        <RingDot tone={dimension.tone} />
        {dimension.label}
      </dt>
      <dd
        className={
          dimension.tone ? `co-360-reading-${dimension.tone}` : undefined
        }
      >
        {/* The label travels INTO the trigger, not only beside it: two
            dimensions reading the same word gave two buttons the same
            accessible name, and the `dt` next to them is not part of it. */}
        <Popover
          onHover
          label={
            <>
              <span className="sr-only">{dimension.label}: </span>
              {dimension.reading}
            </>
          }
        >
          <p className="co-dim-means">{dimension.means}</p>
          {dimension.because ? (
            <>
              {/* The same words the verdict's own grounding uses, over the
                  same quote: two names for one working would read as two
                  readings. A label beside a value, not a heading: the panel
                  is already named by the value that opened it. */}
              <Eyebrow className="co-dim-restson">
                {t("record.restsOn")}
              </Eyebrow>
              <p className="co-dim-quote">{dimension.because}</p>
            </>
          ) : null}
        </Popover>
      </dd>
    </div>
  );
}

/**
 * The 360's call: the head whose mark says a machine read this record, the
 * standing word with the sentence it rests on, the three rated dimensions, and
 * under them whatever the call was read from — the spine, the folded thread.
 *
 * While the read is in flight the card holds its shape and no verdict: a head
 * holding a spinner where the call goes is the reading claiming to have
 * reached one.
 */
export function Company360Call({
  reading,
  name,
  title,
  titleAction,
  scale,
  footer,
  children,
}: Readonly<{
  reading: TodayReading;
  name?: string;
  // Forwarded to `CallCard`; see its own doc. Absent draws the kit's default
  // head for every caller that does not opt in.
  title?: string;
  titleAction?: ReactNode;
  scale?: "record" | "compact";
  footer?: ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  if (reading.state === "loading") {
    return (
      <CallCard name={name} title={title} titleAction={titleAction}>
        <PanelBody>
          <Skeleton width="100%" height={64} />
        </PanelBody>
      </CallCard>
    );
  }
  if (reading.state === "failed") {
    return (
      <CallCard name={name} title={title} titleAction={titleAction}>
        <PanelBody>
          <EmptyState>{t("co.section.unavailable")}</EmptyState>
        </PanelBody>
      </CallCard>
    );
  }
  return (
    <CallCard
      name={name}
      title={title}
      titleAction={titleAction}
      standing={reading.standing}
      because={reading.because}
      restsOn={reading.restsOn}
      scale={scale}
      footer={footer}
    >
      <PanelBody className="co-360-dims">
        <Rings
          label={reading.dimensions
            .map((dimension) => `${dimension.label}: ${dimension.reading}`)
            .join(", ")}
          rings={reading.dimensions.map((dimension) => ({
            key: dimension.key,
            value: dimension.step ?? 0,
            max: HEALTH_RANK.length,
            tone: dimension.tone,
          }))}
        />
        <dl className="co-360-readings">
          {reading.dimensions.map((dimension) => (
            <DimensionChip key={dimension.key} dimension={dimension} />
          ))}
        </dl>
      </PanelBody>
      {children}
    </CallCard>
  );
}

/**
 * What needs a contact: one list, the moment as its lead row, then the agent's
 * finds, then the manual moves. The foot carries what the list as a whole is
 * counting down to and what the advice could not show.
 */
export function NeedsList({
  reading,
  onOpenTasks,
}: Readonly<{
  reading: TodayReading;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own.
  onOpenTasks?: () => void;
}>) {
  if (reading.state !== "ready") {
    return <TodayPanel state={reading.state} onOpenTasks={onOpenTasks} />;
  }
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      footer={reading.footer}
      notice={reading.notice}
      complete={reading.complete}
    >
      {reading.rows}
    </TodayPanel>
  );
}

/**
 * The call and the needs list as one column, for a surface with no glance
 * grid of its own: the two panes in the order the page draws them.
 */
export function TodayOnThisAccount({
  onOpenTasks,
  spine,
  ...inputs
}: TodayReadingInputs &
  Readonly<{
    onOpenTasks?: () => void;
    // The account's story as a thread, under the call it was read from.
    spine?: ReactNode;
  }>) {
  const reading = useTodayReading(inputs);
  return (
    <>
      <Company360Call
        reading={reading}
        name={inputs.view?.company?.display_name}
      >
        {spine}
      </Company360Call>
      <NeedsList reading={reading} onOpenTasks={onOpenTasks} />
    </>
  );
}

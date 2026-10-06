// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a focused Filters and views page. These pages name themselves
// (app/pagemeta.ts, headsItself), so every state one can be in, waiting
// included, prints exactly one h1 from here.

import type { ReactNode } from "react";
import { navigate } from "../app/router";
import { Button, PendingBody } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import "./filters.css";

/** The page's name, what it is, and the controls that act on the whole page. */
export function FocusedHead({
  title,
  facts,
  control,
  actions,
}: Readonly<{
  title: string;
  /** One line saying what the page holds, under its name. */
  facts?: ReactNode;
  /** The one choice everything below reads from, such as the record type. */
  control?: ReactNode;
  actions?: ReactNode;
}>) {
  return (
    <div
      className={control ? "filters-head filters-head-picks" : "filters-head"}
    >
      <div className="filters-head-name">
        <Heading size="xlarge">{title}</Heading>
        {facts && <p className="t-sub">{facts}</p>}
      </div>
      {control}
      {actions && <div className="filters-head-actions">{actions}</div>}
    </div>
  );
}

/**
 * A focused page before it can name itself. It wears the name the trail gives
 * it until then, rather than no heading at all.
 */
export function FocusedPending({
  title,
  label,
}: Readonly<{ title: string; label: string }>) {
  return (
    <div className="wrap filters-screen">
      <FocusedHead title={title} />
      <PendingBody label={label} lines={6} />
    </div>
  );
}

/**
 * A focused page with nothing it can open: what went wrong, under the name the
 * page would have worn, and the way back to where the reader can start again.
 */
export function FocusedState({
  title,
  sentence,
}: Readonly<{ title: string; sentence: string }>) {
  const t = useT();
  return (
    <div className="wrap filters-screen">
      <FocusedHead title={title} />
      <SurfaceState
        state="empty"
        emptyLabel={sentence}
        loadingLabel={t("common.loading")}
      >
        {null}
      </SurfaceState>
      <BackToLibrary />
    </div>
  );
}

/** The way back to where the reader can start again. */
export function BackToLibrary() {
  const t = useT();
  return (
    <p className="filters-back">
      <Button variant="link" onClick={() => navigate({ screen: "filters" })}>
        {t("filters.backToLibrary")}
      </Button>
    </p>
  );
}

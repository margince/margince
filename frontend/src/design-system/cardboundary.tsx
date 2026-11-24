// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryErrorResetBoundary } from "@tanstack/react-query";
import { Component, type ReactNode } from "react";
import { useT } from "../i18n";
import { resetLocaleFailures } from "../i18n/catalogs";
import { retryStaleChunk } from "../lib/stalebuild";
import { Button, EmptyState } from "./atoms";
import "./cardboundary.css";

// CardBoundary contains ONE card's render failure inside that card.
//
// The screen boundary (app/errorboundary.tsx) catches the throw too, but it
// replaces the whole screen. This one replaces only the failed card, which
// offers a retry while the cards beside it keep working.
//
// It never shows the thrown error's text, for the same reason the app-level one
// does not: a render throw is a defect in this app, not a message from the
// server. Its text names our internals, the reader cannot act on it, and a
// stack trace on a settings page is the app leaking rather than explaining.

function CardFailure({
  title,
  retryLabel,
  onRetry,
}: Readonly<{ title: string; retryLabel: string; onRetry: () => void }>) {
  return (
    // role="alert" so the failure is announced where it happened — a reader
    // who is elsewhere on the page when a card gives out is told, without the
    // page stealing their focus to do it. EmptyState draws the recessed card
    // itself, so the boundary keeps the failed card's PLACE in the layout: the
    // grid does not reflow around a hole, which is what a returned null would
    // have made it do.
    <div role="alert">
      <EmptyState>
        <p>{title}</p>
        <Button
          variant="primary"
          onClick={onRetry}
          className="card-failure-retry"
        >
          {retryLabel}
        </Button>
      </EmptyState>
    </div>
  );
}

type BoundaryProps = Readonly<{
  onReset: () => void;
  title: string;
  retryLabel: string;
  children: ReactNode;
  fallback?: ReactNode;
  reloadsOn?: (error: unknown) => boolean;
}>;
type BoundaryState = Readonly<{ failed: boolean; error?: unknown }>;

// React reports a caught error to the console with its component stack on its
// own, so this boundary adds no reporting of its own — the same failure logged
// twice teaches a reader to skim both.
class CardRenderBoundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { failed: false };

  static getDerivedStateFromError(error: unknown): BoundaryState {
    return { failed: true, error };
  }

  private readonly retry = () => {
    if (retryStaleChunk(this.state.error)) return;
    // Not reloadForNewBuild: its guard stops automatic reload loops, and a
    // pressed Retry must always reload.
    if (this.props.reloadsOn?.(this.state.error)) {
      window.location.reload();
      return;
    }
    // The cache is reset BEFORE the re-render: a query that threw keeps its
    // failed result, so a boundary that only cleared its own state would hand
    // the same error straight back and the button would do nothing visible.
    this.props.onReset();
    this.setState({ failed: false, error: undefined });
  };

  render(): ReactNode {
    if (this.state.failed) {
      if (this.props.fallback !== undefined) return this.props.fallback;
      return (
        <CardFailure
          title={this.props.title}
          retryLabel={this.props.retryLabel}
          onRetry={this.retry}
        />
      );
    }
    return this.props.children;
  }
}

/**
 * Wrap ONE card. Paired with the query cache's own reset boundary so the retry
 * clears the failures the cache is holding as well as this boundary's state.
 *
 * The copy is the one exception to "a primitive holds no copy" that this file
 * cannot avoid and does not try to hide: the fallback renders after the card
 * that would have passed the words has already thrown, so it reads the two
 * `card.error*` keys itself rather than taking props that may never arrive.
 * They say nothing about which card this is — the card's own place on the page
 * already does.
 */
export function CardBoundary({
  children,
  fallback,
  reloadsOn,
}: Readonly<{
  children: ReactNode;
  fallback?: ReactNode;
  /** The failures Retry answers with a page reload, such as an import the
   *  browser refused: it keeps a refused module, so drawing again fails again. */
  reloadsOn?: (error: unknown) => boolean;
}>) {
  const t = useT();
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <CardRenderBoundary
          onReset={() => {
            resetLocaleFailures();
            reset();
          }}
          reloadsOn={reloadsOn}
          fallback={fallback}
          title={t("card.errorTitle")}
          retryLabel={t("card.errorRetry")}
        >
          {children}
        </CardRenderBoundary>
      )}
    </QueryErrorResetBoundary>
  );
}

export function SilentCardBoundary({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <CardBoundary fallback={null}>{children}</CardBoundary>;
}

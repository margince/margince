import { QueryErrorResetBoundary, useQueryClient } from "@tanstack/react-query";
import { Component, type ReactNode } from "react";
import { Button, EmptyState } from "../design-system/atoms";
import { useT } from "../i18n";
import { resetLocaleFailures } from "../i18n/catalogs";
import {
  isStaleChunkError,
  reloadForNewBuild,
  retryStaleChunk,
} from "../lib/stalebuild";
import "./errorboundary.css";

// The app-level error boundary (architecture/frontend, "the data layer").
// Without one, a single throw during render unmounts the whole tree and the
// reader is left on a white page with no way back short of a reload they have
// to think of themselves. With one, the throw costs them the view and nothing
// else.

// The fallback never shows the thrown error. A render throw is a defect in
// this app, not a message from the server: its text names our internals, and
// the reader cannot act on it. What they can act on is here.
function RenderFailure({
  onRetry,
  inShell,
}: Readonly<{ onRetry: () => void; inShell: boolean }>) {
  const t = useT();
  return (
    <div
      className={inShell ? "wrap narrow" : "wrap narrow errorboundary-page"}
      role="alert"
    >
      <EmptyState>
        <p>{t("app.errorTitle")}</p>
        <p>{t("app.errorBody")}</p>
        <Button
          variant="primary"
          className="errorboundary-retry"
          onClick={onRetry}
        >
          {t("app.errorRetry")}
        </Button>
      </EmptyState>
    </div>
  );
}

type BoundaryProps = Readonly<{
  onReset: () => void;
  inShell: boolean;
  children: ReactNode;
}>;
type BoundaryState = Readonly<{ failed: boolean; error?: unknown }>;

// React reports a caught error to the console with its component stack on its
// own, so this boundary adds no reporting of its own — the same failure logged
// twice teaches a reader to skim both.
class RenderErrorBoundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { failed: false };

  static getDerivedStateFromError(error: unknown): BoundaryState {
    return { failed: true, error };
  }

  componentDidCatch(error: unknown): void {
    // A screen whose code a deploy replaced is not a defect. A retry cannot
    // load it either; only a reload onto the new build can.
    if (isStaleChunkError(error)) {
      reloadForNewBuild();
    }
  }

  private readonly retry = () => {
    if (retryStaleChunk(this.state.error)) return;
    // The cache is reset BEFORE the re-render: a query that threw keeps its
    // failed result, so a boundary that only cleared its own state would hand
    // the same error straight back and the button would do nothing visible.
    this.props.onReset();
    this.setState({ failed: false, error: undefined });
  };

  render(): ReactNode {
    if (this.state.failed) {
      return (
        <RenderFailure onRetry={this.retry} inShell={this.props.inShell} />
      );
    }
    return this.props.children;
  }
}

// Paired with the query cache's reset boundary.
//
// The reset reaches only a query that throws into render. A view more often
// throws on a cached success, which a remount draws again before any refetch.
// So Retry drops every answer nobody watches, the failed view's among them.
// Only what the view mounts again is fetched, and the shell keeps its own.
function ResettingBoundary({
  children,
  onReset,
  inShell,
}: Readonly<{ children: ReactNode; onReset?: () => void; inShell: boolean }>) {
  const client = useQueryClient();
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <RenderErrorBoundary
          inShell={inShell}
          onReset={() => {
            resetLocaleFailures();
            reset();
            void client.resetQueries({ type: "inactive" });
            onReset?.();
          }}
        >
          {children}
        </RenderErrorBoundary>
      )}
    </QueryErrorResetBoundary>
  );
}

// The last resort, around the whole app: it catches what the shell itself
// throws, and there is no rail or top bar left to keep.
export function AppErrorBoundary({
  children,
  onReset,
}: Readonly<{ children: ReactNode; onReset?: () => void }>) {
  return (
    <ResettingBoundary onReset={onReset} inShell={false}>
      {children}
    </ResettingBoundary>
  );
}

// Around the routed screen inside the shell, so a screen's throw costs the
// reader that screen and the rail still takes them elsewhere. Key it by the
// route: a new address mounts a fresh boundary and drops the failure.
export function ScreenErrorBoundary({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <ResettingBoundary inShell>{children}</ResettingBoundary>;
}

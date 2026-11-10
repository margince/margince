// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
} from "react";
import { scopeKey } from "./analytics.context";
import type { OverviewView } from "./analytics.overview.choice";
import { outboundDials } from "./analytics.overview.context";
import type { ReportingSelection } from "./reporting.model";

export const OverviewNavigation = createContext<
  ((dials: Map<string, string> | undefined) => void) | undefined
>(undefined);

export function useOverviewNavigation(
  view: OverviewView,
  selection: ReportingSelection,
  dates?: Readonly<{ start: string; end: string }>,
) {
  const register = useContext(OverviewNavigation);
  const encoded = new URLSearchParams(
    Array.from(
      outboundDials(view, {
        scope: scopeKey(selection.scope),
        pipeline:
          view === "sales" ? (selection.pipeline_id ?? "all") : undefined,
        period: selection.period,
        close: view === "sales" ? selection.close_window : undefined,
        from: selection.period === "custom" ? dates?.start : undefined,
        through: selection.period === "custom" ? dates?.end : undefined,
        product: selection.product_id,
        initiative: selection.initiative_id,
        country: selection.country,
        industry: selection.industry,
        basis: selection.value_basis,
      }),
    ),
  ).toString();
  useEffect(() => {
    register?.(new Map(new URLSearchParams(encoded)));
    return () => register?.(undefined);
  }, [encoded, register]);
}

export function useOverviewRegistration() {
  const dials = useRef<Map<string, string> | undefined>(undefined);
  const register = useCallback((next: Map<string, string> | undefined) => {
    dials.current = next;
  }, []);
  return { dials, register };
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useCallback, useRef, useState } from "react";
import { useEscapeDismisses } from "../design-system/tooltip";

type TipRaise = "pointer" | "focus";

/** The collapsed rail's one tip at a time, which Escape hides as any tip. */
export function useNavTip(
  collapsed: boolean,
  nav: RefObject<HTMLElement | null>,
) {
  const [tip, setTip] = useState<string | null>(null);
  const raisedBy = useRef<TipRaise>("focus");
  const onTip = useCallback((key: string | null, by?: TipRaise) => {
    if (by !== undefined) {
      raisedBy.current = by;
    }
    setTip(key);
  }, []);
  const hide = useCallback(() => setTip(null), []);
  useEscapeDismisses(collapsed && tip !== null, nav, raisedBy, hide);
  return { collapsed, tip, onTip };
}

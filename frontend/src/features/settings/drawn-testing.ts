// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { isValidElement, type ReactNode } from "react";

/**
 * Whether an element tree names the component `name` anywhere in its children.
 * It reads the tree a settings tab draws without mounting a card or its reads.
 */
export function drawsComponent(node: ReactNode, name: string): boolean {
  const seen = new Set<unknown>();
  const walk = (child: ReactNode): boolean => {
    if (!child || typeof child !== "object") {
      return false;
    }
    if (Array.isArray(child)) {
      return child.some(walk);
    }
    if (seen.has(child)) {
      return false;
    }
    seen.add(child);
    // The props generic lets `children` be read without asserting a shape:
    // isValidElement narrows both halves at once.
    if (!isValidElement<{ children?: ReactNode }>(child)) {
      return false;
    }
    if (typeof child.type === "function" && child.type.name === name) {
      return true;
    }
    return walk(child.props.children);
  };
  return walk(node);
}

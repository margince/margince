// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CellStack } from "./cellstack";
import "./keyedname.css";

/**
 * A thing's human name with the key an operator types or greps for under it,
 * small and in the code face. A key with no name of its own shows once.
 */
export function KeyedName({
  name,
  code,
}: Readonly<{ name: string; code: string }>) {
  if (name === code) {
    return <code className="keyed-name-only">{code}</code>;
  }
  return (
    <CellStack>
      <span>{name}</span>
      <code className="keyed-name-key">{code}</code>
    </CellStack>
  );
}

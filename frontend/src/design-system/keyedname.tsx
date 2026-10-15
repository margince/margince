// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment } from "react";
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
  if (name === code) return <KeyCode code={code} />;
  return (
    <CellStack>
      <span>{name}</span>
      <code className="keyed-name-key">
        <KeyText code={code} />
      </code>
    </CellStack>
  );
}

/** A key on its own, in the code face of the line around it. */
export function KeyCode({ code }: Readonly<{ code: string }>) {
  return (
    <code className="keyed-name-only">
      <KeyText code={code} />
    </code>
  );
}

// A break opportunity after each underscore lets a long key wrap between its
// words; `<wbr>` adds no character, so a copied key still greps.
function KeyText({ code }: Readonly<{ code: string }>) {
  return [...code.matchAll(/(?:[^_]*_)|(?:[^_]+$)/g)].map((part) => (
    <Fragment key={part.index}>
      {part[0]}
      {part[0].endsWith("_") && <wbr />}
    </Fragment>
  ));
}

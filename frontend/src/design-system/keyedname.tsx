// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment } from "react";
import { useT } from "../i18n";
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
  if (name === code) return <BareKey code={code} />;
  return (
    <CellStack>
      <span>
        <KeyText code={name} />
      </span>
      <code className="keyed-name-key">
        <KeyText code={code} />
      </code>
    </CellStack>
  );
}

/**
 * A key on its own under a field, named as a key so a listener hears more
 * than a string of words and underscores.
 */
export function KeyCode({ code }: Readonly<{ code: string }>) {
  const t = useT();
  const [before, after = ""] = t("keyCode.label").split("{key}");
  return (
    <span>
      {before}
      <BareKey code={code} />
      {after}
    </span>
  );
}

function BareKey({ code }: Readonly<{ code: string }>) {
  return (
    <code className="keyed-name-only">
      <KeyText code={code} />
    </code>
  );
}

// A break opportunity after each `_`, `.` or `,` lets a long key, a dotted
// setting name or a JSON value wrap between its words. `<wbr>` adds no
// character, so a copied key still greps.
function KeyText({ code }: Readonly<{ code: string }>) {
  return [...code.matchAll(/(?:[^_.,]*[_.,])|(?:[^_.,]+$)/g)].map((part) => (
    <Fragment key={part.index}>
      {part[0]}
      {/[_.,]$/.test(part[0]) && <wbr />}
    </Fragment>
  ));
}

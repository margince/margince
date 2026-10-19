// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";

/**
 * The row's name at both densities. A link where its subject has a page, a
 * press that opens the subject in place where it has none, else plain text.
 */
export function RowName({
  title,
  href,
  open,
}: Readonly<{ title: string; href?: string; open?: () => void }>) {
  if (href) {
    return (
      <a className="entity-link" href={href}>
        {title}
      </a>
    );
  }
  if (open) {
    return (
      <Button variant="link" aria-haspopup="dialog" onClick={open}>
        {title}
      </Button>
    );
  }
  return <>{title}</>;
}

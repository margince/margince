// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";

export function RowName({
  title,
  href,
  open,
  unavailable,
}: Readonly<{
  title: string;
  href?: string;
  open?: () => void;
  unavailable?: boolean;
}>) {
  const t = useT();
  const name = open ? (
    <Button
      variant="link"
      className="btn-valuelabel"
      aria-haspopup="dialog"
      onClick={open}
    >
      {title}
    </Button>
  ) : href ? (
    <AppLink className="entity-link" href={href}>
      {title}
    </AppLink>
  ) : (
    <span>{title}</span>
  );
  return (
    <>
      {name}
      {unavailable && (
        <>
          {" "}
          · <span className="t-caption">{t("worklist.sourceUnavailable")}</span>
        </>
      )}
    </>
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { GeneratedText } from "../../design-system/generatedtext";
import { useT } from "../../i18n";
import { displayedTitle } from "../../lib/systemdisplay";

export function RowName({
  title,
  item,
  href,
  open,
  unavailable,
}: Readonly<{
  title: string;
  item?: components["schemas"]["WorklistItem"];
  href?: string;
  open?: () => void;
  unavailable?: boolean;
}>) {
  const t = useT();
  const original = item && displayedTitle(item);
  const text =
    item?.source === "task" ? (
      <GeneratedText
        activityId={item.id}
        display={item.system_display}
        inline
        renderText={(translated) =>
          original && title.startsWith(original)
            ? translated + title.slice(original.length)
            : title
        }
      >
        {title}
      </GeneratedText>
    ) : (
      title
    );
  const name = open ? (
    <Button
      variant="link"
      align="start"
      className="btn-valuelabel"
      aria-haspopup="dialog"
      onClick={open}
    >
      {text}
    </Button>
  ) : href ? (
    <AppLink className="entity-link" href={href}>
      {text}
    </AppLink>
  ) : (
    <span>{text}</span>
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

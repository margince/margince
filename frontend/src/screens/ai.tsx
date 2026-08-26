import { useState } from "react";
import { ASK_QUERY_KEY } from "../app/palette";
import { Card, Kbd } from "../design-system/atoms";
import { AutonomyDot } from "../design-system/trust";
import { useT } from "../i18n";
import { CorpusAskCard } from "./corpusask";

// Ask AI (B-EP09.12c, 03b): the BYO-agent surface. Agents connect over MCP
// with a passport; this surface states the two-tier contract honestly —
// 🟢 read/draft executes, 🟡 write/send stages into the approval inbox —
// and never pretends a chat backend exists before one is connected.

export function AskAiScreen() {
  const t = useT();
  const [query] = useState(() => {
    const stored = sessionStorage.getItem(ASK_QUERY_KEY);
    sessionStorage.removeItem(ASK_QUERY_KEY);
    return stored;
  });

  return (
    <div className="wrap">
      {/* The carried question goes straight into the ask box, and the card that
          used to reprint it above an empty box is gone with it. A reader who
          typed a question into the palette ASKED it; showing it back to them
          beside a box they must retype it into was the surface admitting it
          could not answer. */}
      <CorpusAskCard carriedQuestion={query} />
      <Card as="div" title={t("ai.tiers")}>
        <ul
          style={{
            listStyle: "none",
            display: "flex",
            flexDirection: "column",
            gap: 8,
          }}
        >
          <li>
            <AutonomyDot tier="auto" />{" "}
            <strong>{t("ai.tierAutoExecute")}</strong>{" "}
            <span className="t-caption">{t("ai.tierAutoExecuteDetail")}</span>
          </li>
          <li>
            <AutonomyDot tier="confirm" />{" "}
            <strong>{t("ai.tierConfirmationRequired")}</strong>{" "}
            <span className="t-caption">
              {t("ai.tierConfirmationRequiredDetail")}
            </span>
          </li>
        </ul>
      </Card>
      <Card as="div" inset title={t("ai.connect")}>
        <p className="t-caption">{t("ai.connectDetail")}</p>
        <p className="t-caption" style={{ marginTop: 8 }}>
          {t("ai.paletteHint")} <Kbd>⌘K</Kbd>
        </p>
      </Card>
    </div>
  );
}

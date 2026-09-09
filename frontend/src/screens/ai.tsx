import { ASK_QUESTION_PARAM } from "../app/palette";
import { currentParams, replaceParams, useUrlParams } from "../app/urlstate";
import { Card, Kbd } from "../design-system/atoms";
import { AutonomyDot } from "../design-system/trust";
import { useT } from "../i18n";
import { CorpusAskCard } from "./corpusask";

// Ask AI (B-EP09.12c, 03b): the BYO-agent surface. Agents connect over MCP
// with a passport; this surface states the two-tier contract honestly —
// 🟢 read/draft executes, 🟡 write/send stages into the approval inbox —
// and never pretends a chat backend exists before one is connected.

// The question comes out of the address the moment the card has taken it. By
// then it has been asked, so leaving it there would make a reload ask again
// for as long as the reader keeps the tab — and the same question carried a
// second time has to move the address a second time to be a second ask.
//
// Read through `currentParams` rather than a render's snapshot: composing on a
// snapshot is how a second write in one handler discards the first, and this
// one drops a dial rather than setting one.
function forgetCarriedQuestion(): void {
  const dials = new Map(currentParams());
  dials.delete(ASK_QUESTION_PARAM);
  replaceParams(dials);
}

export function AskAiScreen() {
  const t = useT();
  const [dials] = useUrlParams();

  return (
    <div className="wrap">
      {/* A reader who typed a question into the palette ASKED it, so the card
          asks it. Printing it back beside a box they must press would be the
          surface admitting it could not answer. */}
      <CorpusAskCard
        carriedQuestion={dials.get(ASK_QUESTION_PARAM)}
        onCarriedAsked={forgetCarriedQuestion}
      />
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

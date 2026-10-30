// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { FileText, Mail, Mic } from "lucide-react";
import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";
import "./voicekinds.css";

const VOICE_KINDS = [
  {
    key: "mail",
    icon: Mail,
    title: "voice.kinds.mail",
    body: "voice.kinds.mailBody",
    best: true,
  },
  {
    key: "call",
    icon: Mic,
    title: "voice.kinds.call",
    body: "voice.kinds.callBody",
    best: false,
  },
  {
    key: "doc",
    icon: FileText,
    title: "voice.kinds.doc",
    body: "voice.kinds.docBody",
    best: false,
  },
] as const;

/**
 * What writing to bring, before a drop target asks for it. A guide, not three
 * more buttons: every kind arrives through the same browse, paste or drop.
 * The onboarding voice step and the empty settings page both draw it.
 */
export function VoiceKinds() {
  const t = useT();
  return (
    <div className="voicekinds">
      <ul className="voicekinds-list" aria-label={t("voice.kinds.label")}>
        {VOICE_KINDS.map(({ key, icon: Icon, title, body, best }) => (
          <li key={key} className="voicekind">
            <span className="voicekind-icon">
              <Icon aria-hidden />
            </span>
            <b>{t(title)}</b>
            <span className="t-caption">{t(body)}</span>
            {best && <Badge tone="accent">{t("voice.kinds.best")}</Badge>}
          </li>
        ))}
      </ul>
    </div>
  );
}

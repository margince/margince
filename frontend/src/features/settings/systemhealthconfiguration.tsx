// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";

export default function SystemHealthConfiguration() {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  if (!canRead) return <p>{t("health.configurationWithheld")}</p>;
  return (
    <>
      <BackgroundSchedulesCard />
      <SendPacingCard />
    </>
  );
}

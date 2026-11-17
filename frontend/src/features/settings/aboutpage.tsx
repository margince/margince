// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AboutMargince } from "../../components/aboutmargince";
import { Panel, PanelBody } from "../../design-system/panel";
import { buildRelease } from "../../lib/buildrelease";
import "./aboutpage.css";

export function AboutPage() {
  return (
    <Panel>
      <PanelBody>
        <div className="aboutpage">
          <AboutMargince build={buildRelease()} />
        </div>
      </PanelBody>
    </Panel>
  );
}

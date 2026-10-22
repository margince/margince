import { type ExtensionPanelProps, useT } from "@margince/frontend/app";
import { Card, Panel, PanelBody } from "@margince/frontend/design-system";

// The smallest frontend layer: the screen every unit package's main exports,
// and one panel on the deal page. The panel gives the composed panel path a
// declarer.

export default function CrmHelloScreen() {
  const t = useT();
  return (
    <Card title={t("extCrmHello.title")}>
      <p>{t("extCrmHello.sub")}</p>
    </Card>
  );
}

export function DealHello({ record }: ExtensionPanelProps) {
  const t = useT();
  return (
    <Panel title={t("extCrmHello.title")}>
      <PanelBody>
        <p>{t("extCrmHello.panel", { deal: record.id })}</p>
      </PanelBody>
    </Panel>
  );
}

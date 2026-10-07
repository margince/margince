// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func reportingRetentionSelector() string {
	p := strings.Split(storekit.Placeholders([]int{0, 0}), ",")
	return `SELECT edition.id FROM report_edition edition
 WHERE edition.expired_at IS NULL AND edition.captured_at < now() - make_interval(days => ` + p[0] + `)
 AND NOT EXISTS(SELECT 1 FROM report_edition_contribution c
 LEFT JOIN deal d ON c.source_type='deal' AND d.id=c.source_id
 LEFT JOIN company dc ON dc.id=d.company_id
 LEFT JOIN sdr_handoff h ON c.source_type='sdr_handoff' AND h.id=c.source_id
 LEFT JOIN contact hc ON hc.id=h.contact_id
 LEFT JOIN lead hl ON hl.id=h.lead_id
 LEFT JOIN company ho ON ho.id=h.company_id
 LEFT JOIN deal hd ON hd.id=h.deal_id
 WHERE c.edition_id=edition.id AND (
 COALESCE(d.legal_hold,false) OR COALESCE(dc.legal_hold,false) OR COALESCE(hc.legal_hold,false)
 OR COALESCE(hl.legal_hold,false) OR COALESCE(ho.legal_hold,false) OR COALESCE(hd.legal_hold,false)
 OR EXISTS(SELECT 1 FROM relationship rel JOIN contact subject ON subject.id=rel.contact_id
 WHERE rel.deal_id=d.id AND rel.kind='deal_stakeholder' AND subject.legal_hold)
 OR (c.source_type='activity' AND EXISTS(SELECT 1 FROM activity_link l
 LEFT JOIN contact ac ON ac.id=l.contact_id LEFT JOIN company ao ON ao.id=l.company_id
 LEFT JOIN deal ad ON ad.id=l.deal_id LEFT JOIN lead al ON al.id=l.lead_id LEFT JOIN project ap ON ap.id=l.project_id
 WHERE l.activity_id=c.source_id AND (COALESCE(ac.legal_hold,false) OR COALESCE(ao.legal_hold,false)
 OR COALESCE(ad.legal_hold,false) OR COALESCE(al.legal_hold,false) OR COALESCE(ap.legal_hold,false))))))
 ORDER BY edition.captured_at,edition.id LIMIT ` + p[1]
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// travelWords name travel logistics in a meeting subject, in English, German
// and Vietnamese. Each matches as a whole word or phrase, any case. Some words
// also name business: "transfer", "train", "hotel", "Zug", "tàu", "đón". Those
// count only inside a travel phrase, so "Knowledge transfer" still asks.
var travelWords = []string{
	"flight", "flights", "airport", "drop-off", "drop off", "dropoff",
	"pickup", "pick-up", "pick up", "taxi", "train station", "train ride",
	"airport transfer", "hotel transfer", "hotel check-in", "hotel check-out",
	"flug", "flüge", "abflug", "flughafen", "abholung", "bahnhof", "zugfahrt",
	"chuyến bay", "sân bay", "đưa đón", "ga tàu", "tàu hỏa",
	"nhận phòng khách sạn", "trả phòng khách sạn",
}

// travelSubjectPattern is travelWords as one PostgreSQL pattern. A word must
// stand between non-letters, so "Flug" does not match "Flugzeugbau".
var travelSubjectPattern = func() string {
	quoted := make([]string, len(travelWords))
	for i, word := range travelWords {
		quoted[i] = regexp.QuoteMeta(word)
	}
	return `(^|[^[:alnum:]])(` + strings.Join(quoted, "|") + `)([^[:alnum:]]|$)`
}()

// travelLogisticsClause is a meeting under alias a whose subject names travel
// logistics and that links no deal, lead or project this reader may see. Such
// a meeting owes no outcome; a linked one is business held while travelling.
func travelLogisticsClause(ctx context.Context, arg func(any) int) (string, error) {
	visible, err := auth.LinkTargetReadableClause(ctx, "al", arg)
	if err != nil {
		return "", err
	}
	if visible == "" {
		visible = scopeUnbounded
	}
	return sprintf(`(COALESCE(a.subject ~* $%d::text, false) AND NOT EXISTS (SELECT 1 FROM activity_link al
	   WHERE al.activity_id = a.id AND (al.deal_id IS NOT NULL OR al.lead_id IS NOT NULL OR al.project_id IS NOT NULL)
	   AND (%s)))`, arg(travelSubjectPattern), visible), nil
}

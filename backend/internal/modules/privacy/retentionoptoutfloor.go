// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A refusal of advertising and its receipt carry their own retention floor,
// kept_until, stamped by consent from the subject's messaging rules. No
// retention policy anonymizes, erases or archives a record while one stands.

// optOutRecordsKept holds while the record whose activity_link column is
// column, matched by idExpr, has a refusal or a receipt inside its floor.
func optOutRecordsKept(column, idExpr string) string {
	return `(EXISTS (SELECT 1 FROM communication_suppression ks
	          WHERE ks.` + column + ` = ` + idExpr + ` AND ks.kept_until > now())
	  OR EXISTS (SELECT 1 FROM comms_outbound ko JOIN activity_link kl ON kl.activity_id = ko.activity_id
	          WHERE kl.` + column + ` = ` + idExpr + ` AND ko.kept_until > now()))`
}

// receiptKept holds while the activity matched by activityExpr is a receipt
// inside its floor.
func receiptKept(activityExpr string) string {
	return `EXISTS (SELECT 1 FROM comms_outbound ko WHERE ko.activity_id = ` + activityExpr + ` AND ko.kept_until > now())`
}

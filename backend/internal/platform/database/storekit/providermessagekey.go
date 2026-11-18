// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// MailboxMarkSQL names one capture connection over alias and the generation of
// the account it is bound to: "<mailbox>:<generation>". A row made while a
// member held one mailbox per provider uses the provider, so the keys it
// stored still match. Every later row uses its id, so two mailboxes of one
// provider never share a mark.
func MailboxMarkSQL(alias string) string {
	return "(" + MailboxSQL(alias) + " || ':' || " + alias + ".generation)"
}

// MailboxSQL is the mailbox half of MailboxMarkSQL.
func MailboxSQL(alias string) string {
	return "(CASE WHEN " + alias + ".keys_by_provider THEN " + alias + ".provider ELSE " + alias + ".id::text END)"
}

// ProviderMessageKeysSQL is the keys an opaque provider id has under the
// seat's live connections of provider. A connection that is not NULL narrows
// them to that one. Every argument is an internally derived SQL placeholder.
func ProviderMessageKeysSQL(seat, provider, connection, message string) string {
	return ConnectionMessageKeysSQL("c.user_id = "+seat+" AND c.provider = "+provider+
		" AND ("+connection+"::uuid IS NULL OR c.id = "+connection+"::uuid)", message)
}

// ConnectionMessageKeysSQL is ProviderMessageKeysSQL for the live connection
// match names over alias c, such as a shared mailbox by its id.
func ConnectionMessageKeysSQL(match, message string) string {
	return "ARRAY(SELECT " + MailboxMarkSQL("c") + " || ':' || " + message +
		" FROM capture_connection c WHERE " + match +
		" AND c.archived_at IS NULL AND c.status IN ('connected','error') AND " + message + " <> '')"
}

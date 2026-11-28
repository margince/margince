// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCompanyMergeKeepsTheSurvivorChannelAndRetiresTheOtherBinding(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	from, to := seedMergePair(t, e, "Chat rooms")
	connection, sourceLink, targetLink := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO channel_connection (id,provider,channel_id,channel_label,address,credential_ref,status,connected_by)
 VALUES ($1,'dispact','merge-rooms','Rooms','https://chat.example.test','test-key','connected',$2)`, connection, e.AdminUser)
	for _, row := range []struct {
		id      ids.UUID
		company ids.CompanyID
		slug    string
	}{{sourceLink, from, "source-room"}, {targetLink, to, "survivor-room"}} {
		e.WsExec(t, `INSERT INTO chat_link (id,connection_id,provider,channel_slug,channel_name,company_id,linked_by)
 VALUES ($1,$2,'dispact',$3,$3,$4,$5)`, row.id, connection, row.slug, row.company, e.AdminUser)
	}
	if _, err := e.Contacts.MergeCompany(admin, from, to, nil); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM chat_link WHERE company_id=$1 AND archived_at IS NULL`, to); n != 1 {
		t.Fatalf("survivor live bindings: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM chat_link WHERE id=$1 AND company_id=$2 AND archived_at IS NOT NULL`, sourceLink, to); n != 1 {
		t.Fatal("source association was not retired and moved")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM chat_link WHERE id=$1 AND archived_at IS NULL`, targetLink); n != 1 {
		t.Fatal("survivor association was replaced")
	}
}

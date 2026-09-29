// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/jackc/pgx/v5/pgxpool"

// WithListsEnabled switches Live Lists and Shortlists on for this server: the
// /lists routes, the list_id narrowing of the record lists, the list export
// source, the company page's memberships and the agent list tools. cmd/api
// reads lists.enabled from the deployment file and passes it here.
func WithListsEnabled(on bool) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.listsEnabled = on
	}
}

// publishListsAvailability hands the one switch to every surface it gates.
// It runs after the option loop, so a server built without the option answers
// every one of them as off, together.
func (s *Server) publishListsAvailability(pool *pgxpool.Pool) {
	s.collectionsHandlers = s.WithListsEnabled(s.listsEnabled)
	s.authHandlers = s.WithListsAvailable(s.listsEnabled)
	s.listsOn = s.listsEnabled
	if !s.listsEnabled {
		return
	}
	lists := NewCollectionsStore(pool)
	s.contactsHandlers = s.contactsHandlers.WithListMembers(lists.MemberFilter)
	s.dealsHandlers = s.dealsHandlers.WithListMembers(lists.MemberFilter)
	s.bulkHandlers.engine.withLists(lists)
	if s.company360Svc != nil {
		s.company360Svc.ShowListMemberships()
	}
}

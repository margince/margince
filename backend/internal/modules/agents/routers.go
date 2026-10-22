// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// RegisterRouters registers the routers whose targets come from several
// Register* calls, so it runs after all of them. A router whose targets share
// one seam registers beside them, and is absent when that seam is.
func RegisterRouters(r *Registry) {
	registerWhereServed(r, changeState{})
	registerWhereServed(r, readWorkspace{})
	r.Register(readForecast{})
	registerWhereServed(r, findRecords{})
	registerWhereServed(r, readContext{})
	registerWhereServed(r, readAttention{})
	registerWhereServed(r, readReports{})
	registerWhereServed(r, readAnalytics{})
	registerWhereServed(r, changeActivities{})
	registerWhereServed(r, scheduleMeeting{})
	registerWhereServed(r, draftOutbound{})
	registerWhereServed(r, readProject{})
	registerWhereServed(r, resolveDuplicates{})
}

// registerWhereServed registers a router whose targets come from seams a
// composition may leave out, and only where every target was registered.
func registerWhereServed(r *Registry, rt router) {
	for _, target := range rt.routeTargets() {
		if _, served := r.Spec(target); !served {
			return
		}
	}
	r.Register(rt)
}

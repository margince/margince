// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

// KindPersonal is the verdict kind of a thread about the mailbox owner's own
// life: family, friends, a doctor, a landlord.
const KindPersonal = "personal"

// NoPersonalHoldSQL is true while no importing seat holds the thread of
// activity alias as personal.
func NoPersonalHoldSQL(alias string) string {
	return noImporterVerdictSQL(alias, "thread_hold.kind = '"+KindPersonal+"' AND "+HoldsSQL("thread_hold.status"))
}

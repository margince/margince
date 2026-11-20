// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// Provision is the law a refusal or finding rests on, as a pack that binds the
// message cites it. It is shown beside the engine's sentence and decides nothing.
// The zero value is no citation, which the surface reads as "show the facts".
type Provision struct {
	Instrument string `json:"instrument"`
	Provision  string `json:"provision"`
	SourceURL  string `json:"source_url"`
}

// Cited reports whether a provision names any law.
func (p Provision) Cited() bool { return p.Instrument != "" }

// ProvisionFor is the first provision this decision cites for code, the
// recipient's pack's when it cites one, or the zero value. It reads only what
// the engine resolved when it decided.
func (d Decision) ProvisionFor(code string) Provision {
	if cited := d.Citations[code]; len(cited) > 0 {
		return cited[0]
	}
	return Provision{}
}

// ProvisionsFor is every provision this decision cites for code: the
// recipient's pack's first, then the sending entity's.
func (d Decision) ProvisionsFor(code string) []Provision {
	return d.Citations[code]
}

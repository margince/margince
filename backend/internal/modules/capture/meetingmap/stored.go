// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import "github.com/margince/margince/backend/internal/shared/ports/connector"

// RecordOf maps one stored event resource to the record live capture writes.
// A repair then reads a stored event the way a pull reads it.
func RecordOf(raw []byte, owner, connectorName string, decode Decode) (connector.NormalizedRecord, error) {
	ev, err := decode(raw, owner)
	if err != nil {
		return connector.NormalizedRecord{}, err
	}
	return Classify(ev, owner).ToRecord(connectorName, raw), nil
}

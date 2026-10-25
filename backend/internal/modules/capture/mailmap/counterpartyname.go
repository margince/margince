// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"fmt"
	"strings"

	"github.com/emersion/go-message/mail"
)

// CounterpartyNameOf reads the display name one stored original gave its
// counterparty address. It reads the list Parse takes that name from: From on
// inbound mail, To on outbound. found is false when that list does not hold
// the address.
func CounterpartyNameOf(raw []byte, address string, outbound bool) (name string, found bool, err error) {
	root, err := readEntity(raw)
	if err != nil {
		return "", false, fmt.Errorf("mailmap: parsing message: %w", err)
	}
	header := mail.Header{Header: root.Header}
	list, _ := addressList(header, "From")
	if groupOriginal, viaGroup := googleGroupSender(header); viaGroup {
		list = groupOriginal
	}
	if outbound {
		list, _ = addressList(header, "To")
	}
	for _, a := range list {
		if strings.EqualFold(strings.TrimSpace(a.Address), strings.TrimSpace(address)) {
			return displayName(list, a.Address), true, nil
		}
	}
	return "", false, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
 "context"
 "github.com/jackc/pgx/v5"
)

// PaymentAgeReader applies the Home grouping age inside its finance read.
type PaymentAgeReader func(context.Context, pgx.Tx) (int, error)

// WithPaymentAge binds the installation's grouping preference.
func (h Handlers) WithPaymentAge(read PaymentAgeReader) Handlers {
 h.store.paymentAge = read
 return h
}

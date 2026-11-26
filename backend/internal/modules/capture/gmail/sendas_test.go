// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// sendAsStub is the stub Google with a send-as settings answer of its own.
func sendAsStub(t *testing.T, sendAs http.HandlerFunc) *http.ServeMux {
	t.Helper()
	mux := fullStub(t, nil)
	mux.HandleFunc("/settings/sendAs", sendAs)
	return mux
}

func TestTheSendAsSignatureIsTheDefaultAddresses(t *testing.T) {
	c := realConn(t, sendAsStub(t, func(w http.ResponseWriter, _ *http.Request) {
		writeStub(w, map[string]any{"sendAs": []map[string]any{
			{"sendAsEmail": "rep@myco.com", "isPrimary": true, "signature": "<b>Primary</b>"},
			{"sendAsEmail": "sales@myco.com", "isDefault": true, "signature": "<b>Sales desk</b>"},
		}})
	}))
	got, err := c.SendAsSignature(context.Background(), authBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != "sales@myco.com" || got.HTML != "<b>Sales desk</b>" {
		t.Errorf("signature = %+v, want the default send-as address's", got)
	}
}

func TestASendAsReadGoogleRefusesIsAnAuthRefusal(t *testing.T) {
	c := realConn(t, sendAsStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	if _, err := c.SendAsSignature(context.Background(), authBytes(t)); !errors.Is(err, ErrAuthRejected) {
		t.Errorf("a refused read answered %v, want ErrAuthRejected", err)
	}
}

func TestOnlyAScopeThatReadsSettingsCanReadTheSignature(t *testing.T) {
	if CanReadSendAs([]string{SendScope}) {
		t.Error("the send-only scope was taken as able to read the send-as settings")
	}
	if !CanReadSendAs([]string{SendScope, sharedScope}) {
		t.Error("gmail.readonly was not taken as able to read the send-as settings")
	}
}

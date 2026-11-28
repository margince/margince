// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	probes := []struct {
		shape reflect.Type
		call  func([]byte) error
	}{
		{reflect.TypeFor[crmcontracts.MessengerSendRequest](), func(raw []byte) error {
			var in crmcontracts.MessengerSendRequest
			if err := json.Unmarshal(raw, &in); err != nil {
				return err
			}
			_, err := messengerSendInput(in)
			return err
		}},
		{reflect.TypeFor[crmcontracts.ReviewChannelDeliveryRequest](), func(raw []byte) error {
			var in crmcontracts.ReviewChannelDeliveryRequest
			if err := json.Unmarshal(raw, &in); err != nil {
				return err
			}
			_, err := deliveryReviewInput(in)
			return err
		}},
		{reflect.TypeFor[crmcontracts.ConnectTeamsChannelsJSONRequestBody](), func(raw []byte) error {
			var in crmcontracts.ConnectTeamsChannelsJSONRequestBody
			if err := json.Unmarshal(raw, &in); err != nil {
				return err
			}
			_, err := TeamInstallationID(in)
			return err
		}},
	}
	for _, probe := range probes {
		fields := []string{}
		for field := range probe.shape.Fields() {
			name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			if field.Type == reflect.TypeFor[openapi_types.UUID]() && name != "" && !strings.Contains(options, "omitempty") {
				fields = append(fields, name)
			}
		}
		if len(fields) == 0 {
			t.Fatalf("%s has no required body IDs", probe.shape.Name())
		}
		for _, omitted := range fields {
			t.Run(probe.shape.Name()+"/"+omitted, func(t *testing.T) {
				body := map[string]string{}
				for _, field := range fields {
					if field != omitted {
						body[field] = ids.NewV7().String()
					}
				}
				raw, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				faulttest.AssertNamesOmittedID(t, probe.call(raw), omitted)
			})
		}
	}
}

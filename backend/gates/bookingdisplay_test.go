// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

func attendeeDeliveryMention(fn *ast.FuncDecl) bool {
	body := fn.Body
	if projectionNamesShadowed(fn) {
		return namesAttendees(body)
	}
	display := map[*ast.SelectorExpr]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BinaryExpr:
			if n.Op == token.NEQ || n.Op == token.EQL {
				if id, ok := n.Y.(*ast.Ident); ok && id.Name == "nil" {
					if field, ok := n.X.(*ast.SelectorExpr); ok {
						display[field] = true
					}
				}
			}
		case *ast.RangeStmt:
			if ranged, ok := n.X.(*ast.StarExpr); ok && intendedAttendeeProjection(n.Body) {
				if field, ok := ranged.X.(*ast.SelectorExpr); ok {
					display[field] = true
				}
			}
		}
		return true
	})
	delivery := false
	ast.Inspect(body, func(node ast.Node) bool {
		if field, ok := node.(*ast.SelectorExpr); ok && field.Sel.Name == attendeeField && !display[field] {
			delivery = true
		}
		return !delivery
	})
	return delivery
}

// A projection may append display values, but cannot pass addresses to another
// call or retain them in another assignment for a later send.
func intendedAttendeeProjection(body *ast.BlockStmt) bool {
	valid, caption := true, false
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			name, ok := n.Fun.(*ast.Ident)
			if !ok {
				valid = false
				break
			}
			switch name.Name {
			case "append":
				if len(n.Args) != 2 {
					valid = false
					break
				}
				detail, ok := n.Args[1].(*ast.CallExpr)
				if !ok {
					valid = false
					break
				}
				builder, ok := detail.Fun.(*ast.Ident)
				if !ok || builder.Name != "previewDetail" {
					valid = false
				}
			case "string":
			case "previewDetail":
				if len(n.Args) != 2 {
					valid = false
					break
				}
				literal, ok := n.Args[0].(*ast.BasicLit)
				if !ok {
					valid = false
					break
				}
				label, err := strconv.Unquote(literal.Value)
				if err != nil || label != "intended_attendee" {
					valid = false
					break
				}
				caption = true
			default:
				valid = false
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
				valid = false
				break
			}
			if _, ok := n.Lhs[0].(*ast.Ident); !ok {
				valid = false
				break
			}
			call, ok := n.Rhs[0].(*ast.CallExpr)
			if !ok {
				valid = false
				break
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "append" {
				valid = false
			}
		case *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt, *ast.ReturnStmt, *ast.DeclStmt, *ast.FuncLit:
			valid = false
		}
		return valid
	})
	return valid && caption
}

func TestBookingAddressUsesDistinguishDisplayFromDelivery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		delivery   bool
	}{
		{"display", `if in.AttendeeEmails != nil { for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))) } }`, false},
		{"send within projection", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))); mailer.Send(address) }`, true},
		{"copied list", `addresses := in.AttendeeEmails; transport.Invite(addresses)`, true},
		{"whole list", `transport.Invite(in.AttendeeEmails)`, true},
		{"wrong caption", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("attendee", string(address))) }`, true},
		{"saved address list", `for _, address := range *in.AttendeeEmails { saved = append(saved, address); details = append(details, previewDetail("intended_attendee", string(address))) }`, true},
		{"saved address", `for _, address := range *in.AttendeeEmails { saved = address; details = append(details, previewDetail("intended_attendee", string(address))) }`, true},
		{"returned address", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))); return address }`, true},
		{"shadowed builder", `previewDetail := transport.Send; for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))) }`, true},
		{"shadowed append", `append := transport.Send; for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))) }`, true},
		{"channel send", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))); ch <- address }`, true},
		{"goroutine", `for _, address := range *in.AttendeeEmails { go previewDetail("intended_attendee", string(address)) }`, true},
		{"deferred call", `for _, address := range *in.AttendeeEmails { defer previewDetail("intended_attendee", string(address)) }`, true},
		{"variable declaration", `for _, address := range *in.AttendeeEmails { var saved = address; details = append(details, previewDetail("intended_attendee", string(address))) }`, true},
		{"closure", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(func() string {return address}()))) }`, true},
		{"extra append argument", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address)), address) }`, true},
		{"missing detail value", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee")) }`, true},
		{"dynamic caption", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail(label, string(address))) }`, true},
		{"display and separate send", `for _, address := range *in.AttendeeEmails { details = append(details, previewDetail("intended_attendee", string(address))) }; transport.Invite(in.AttendeeEmails)`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func check() {"+tc.body+"}", 0)
			if err != nil {
				t.Fatal(err)
			}
			fn, ok := file.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatal("fixture has no function")
			}
			if got := attendeeDeliveryMention(fn); got != tc.delivery {
				t.Fatalf("delivery=%t want %t", got, tc.delivery)
			}
		})
	}
}

func projectionNamesShadowed(fn *ast.FuncDecl) bool {
	shadowed := false
	binding := func(id *ast.Ident) {
		switch id.Name {
		case "previewDetail", "append", "string":
			shadowed = true
		}
	}
	ast.Inspect(fn, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Field:
			for _, id := range n.Names {
				binding(id)
			}
		case *ast.ValueSpec:
			for _, id := range n.Names {
				binding(id)
			}
		case *ast.TypeSpec:
			binding(n.Name)
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					binding(id)
				}
			}
		case *ast.RangeStmt:
			if id, ok := n.Key.(*ast.Ident); ok {
				binding(id)
			}
			if id, ok := n.Value.(*ast.Ident); ok {
				binding(id)
			}
		}
		return !shadowed
	})
	return shadowed
}

func TestBookingProjectionRefusesShadowedParameters(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"previewDetail", "append", "string"} {
		t.Run(name, func(t *testing.T) {
			source := `package fixture; func check(` + name + ` func()) {
    for _, address := range *in.AttendeeEmails {
     details = append(details, previewDetail("intended_attendee", string(address)))
    }
   }`
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			fn, ok := file.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatal("fixture has no function")
			}
			if !attendeeDeliveryMention(fn) {
				t.Fatal("shadowed parameter hid an address consumer")
			}
		})
	}
}

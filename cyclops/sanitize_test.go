package cyclops

import "fmt"
import "strings"
import "testing"

func TestIdent(t *testing.T) {
	valid := []string{"users", "rec1", "foo.object", "coalition_slavic_lit", "a.b.c", "foo."}
	for _, s := range valid {
		if _, err := ident("set", s); err != nil {
			t.Errorf("ident(%q) unexpectedly rejected: %v", s, err)
		}
	}

	// Each of these would enable statement injection or otherwise break out
	// of an identifier position, and must be rejected. The error message must
	// name the kind of identifier (the caption) and quote the offending value.
	invalid := []string{
		"",
		"users; drop set secret",
		"users where 1=1",
		"foo bar",
		"foo'",
		"foo)",
		".foo",
		"palci-cultural",
		"1rec",
	}
	for _, s := range invalid {
		_, err := ident("project", s)
		if err == nil {
			t.Errorf("ident(%q) should have been rejected", s)
			continue
		}
		want := fmt.Sprintf("invalid project identifier: %q", s)
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ident(%q) error = %q, want it to contain %q", s, err.Error(), want)
		}
	}
}

func TestIntval(t *testing.T) {
	if v, err := intval("100"); err != nil || v != "100" {
		t.Errorf("intval(\"100\") = %q, %v", v, err)
	}
	if v, err := intval("007"); err != nil || v != "007" {
		t.Errorf("intval(\"007\") = %q, %v; want unchanged \"007\"", v, err)
	}
	if v, err := intval("999999999999999999999"); err != nil || v != "999999999999999999999" {
		t.Errorf("intval(large) = %q, %v; want unchanged (no int64 bound)", v, err)
	}
	for _, s := range []string{"", "10; drop set x", "1.5", "abc", "+7"} {
		_, err := intval(s)
		if err == nil {
			t.Errorf("intval(%q) should have been rejected", s)
			continue
		}
		want := fmt.Sprintf("invalid integer: %q", s)
		if !strings.Contains(err.Error(), want) {
			t.Errorf("intval(%q) error = %q, want it to contain %q", s, err.Error(), want)
		}
	}
}

func TestSQLString(t *testing.T) {
	cases := map[string]string{
		"":                  "''",
		"Mike's Project":    "'Mike''s Project'",
		"plain":             "'plain'",
		"'; drop set x; --": "'''; drop set x; --'", // leading quote doubled, stays inside the literal
		"a'b'c":             "'a''b''c'",
	}
	for in, want := range cases {
		got, err := sqlString(in)
		if err != nil {
			t.Errorf("sqlString(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("sqlString(%q) = %q, want %q", in, got, want)
		}
	}

	for _, s := range []string{"line1\nline2", "with\rCR", "nul\x00byte"} {
		if _, err := sqlString(s); err == nil {
			t.Errorf("sqlString(%q) should have rejected control character", s)
		}
	}
}

func TestFieldList(t *testing.T) {
	cases := map[string]string{
		"id,name":  "id,name",
		"id, name": "id,name",
		"*":        "*",
		"COUNT(*)": "COUNT(*)",
	}
	for in, want := range cases {
		got, err := fieldList(in)
		if err != nil || got != want {
			t.Errorf("fieldList(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	_, err := fieldList("id; drop set x")
	if err == nil {
		t.Errorf("fieldList with injection should have been rejected")
	} else if want := `invalid field: "id; drop set x"`; !strings.Contains(err.Error(), want) {
		t.Errorf("fieldList injection error = %q, want it to contain %q", err.Error(), want)
	}
}

func TestSortList(t *testing.T) {
	cases := map[string]string{
		"name":          "name",
		"name asc":      "name asc",
		"name DESC":     "name desc",
		"name, id desc": "name,id desc",
	}
	for in, want := range cases {
		got, err := sortList(in)
		if err != nil || got != want {
			t.Errorf("sortList(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	rejected := map[string]string{
		"name; drop set x": `invalid sort term: "name; drop set x"`,
		"name sideways":    `invalid sort direction: "sideways"`,
		"name asc desc":    `invalid sort term: "name asc desc"`,
	}
	for s, want := range rejected {
		_, err := sortList(s)
		if err == nil {
			t.Errorf("sortList(%q) should have been rejected", s)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("sortList(%q) error = %q, want it to contain %q", s, err.Error(), want)
		}
	}
}

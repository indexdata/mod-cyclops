package cyclops

import "testing"

func TestIdent(t *testing.T) {
	valid := []string{"users", "rec1", "foo.object", "coalition_slavic_lit", "palci-cultural", "a.b.c"}
	for _, s := range valid {
		if _, err := ident(s); err != nil {
			t.Errorf("ident(%q) unexpectedly rejected: %v", s, err)
		}
	}

	// Each of these would enable statement injection or otherwise break out
	// of an identifier position, and must be rejected.
	invalid := []string{
		"",
		"users; drop set secret",
		"users where 1=1",
		"foo bar",
		"foo'",
		"foo)",
		".foo",
		"foo.",
	}
	for _, s := range invalid {
		if _, err := ident(s); err == nil {
			t.Errorf("ident(%q) should have been rejected", s)
		}
	}
}

func TestIntval(t *testing.T) {
	if v, err := intval("100"); err != nil || v != "100" {
		t.Errorf("intval(\"100\") = %q, %v", v, err)
	}
	if v, err := intval("007"); err != nil || v != "7" {
		t.Errorf("intval(\"007\") = %q, %v; want canonical \"7\"", v, err)
	}
	for _, s := range []string{"", "10; drop set x", "1.5", "abc"} {
		if _, err := intval(s); err == nil {
			t.Errorf("intval(%q) should have been rejected", s)
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
	if _, err := fieldList("id; drop set x"); err == nil {
		t.Errorf("fieldList with injection should have been rejected")
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
	for _, s := range []string{"name; drop set x", "name sideways", "name asc desc"} {
		if _, err := sortList(s); err == nil {
			t.Errorf("sortList(%q) should have been rejected", s)
		}
	}
}

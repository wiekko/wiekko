package ops

import (
	"errors"
	"testing"
)

type item struct{ id, name string }

func TestMatch(t *testing.T) {
	items := []item{
		{"0f320e9a-1111-4000-8000-000000000001", "Quick Entry"},
		{"0f32aaaa-2222-4000-8000-000000000002", "Photo Attachment"},
		{"7a000000-3333-4000-8000-000000000003", "Offline Sync"},
	}
	id := func(i item) string { return i.id }
	label := func(i item) string { return i.name }
	byName := func(ref string) func(item) bool {
		return func(i item) bool { return i.name == ref }
	}

	tests := []struct {
		ref     string
		want    string
		wantErr any
	}{
		{ref: "0f320e9a-1111-4000-8000-000000000001", want: "Quick Entry"},
		{ref: "0F320E9A", want: "Quick Entry"}, // prefix, case-insensitive
		{ref: "7a00", want: "Offline Sync"},
		{ref: "Photo Attachment", want: "Photo Attachment"},
		{ref: "0f32", wantErr: &AmbiguousError{}},
		{ref: "7a0", wantErr: &NotFoundError{}}, // prefixes need 4 characters
		{ref: "nope", wantErr: &NotFoundError{}},
		{ref: "", wantErr: &NotFoundError{}},
	}
	for _, tt := range tests {
		got, err := match("thing", tt.ref, items, id, label, byName(tt.ref))
		switch want := tt.wantErr.(type) {
		case nil:
			if err != nil || got.name != tt.want {
				t.Errorf("match(%q) = %q, %v; want %q", tt.ref, got.name, err, tt.want)
			}
		case *AmbiguousError:
			if !errors.As(err, &want) || len(want.Matches) != 2 {
				t.Errorf("match(%q) err = %v, want ambiguous with 2 matches", tt.ref, err)
			}
		case *NotFoundError:
			if !errors.As(err, &want) {
				t.Errorf("match(%q) err = %v, want not found", tt.ref, err)
			}
		}
	}
}

func TestShortIDAndUser(t *testing.T) {
	if got := ShortID("0f320e9a-1111"); got != "0f320e9a" {
		t.Errorf("ShortID = %q", got)
	}
	if got := ShortID("abc"); got != "abc" {
		t.Errorf("ShortID(short) = %q", got)
	}
	names := map[string]string{"u1": "ada"}
	if user(names, "u1") != "ada" || user(names, "") != "" || user(names, "0f320e9a-1111") != "user:0f320e9a" {
		t.Errorf("user() mapping wrong")
	}
}

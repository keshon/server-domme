package mind

import (
	"reflect"
	"testing"
)

func TestResolveMentions(t *testing.T) {
	people := []Person{
		{ID: "1", Name: "Big M"},
		{ID: "2", Name: "Big Mike"},
		{ID: "3", Name: "cass"},
	}
	cases := []struct {
		in, want string
		ids      []string
	}{
		{"@Big M hello butthead", "<@1> hello butthead", []string{"1"}},
		{"@big m, again?", "<@1>, again?", []string{"1"}},
		// The longer name wins where both would fit.
		{"@Big Mike you too", "<@2> you too", []string{"2"}},
		// A name has to end where it ends.
		{"@Big Mama is not here", "@Big Mama is not here", nil},
		{"@cass and @cass", "<@3> and <@3>", []string{"3"}},
		{"nobody@example.com", "nobody@example.com", nil},
		{"@stranger hi", "@stranger hi", nil},
		{"no mentions at all", "no mentions at all", nil},
	}
	for _, c := range cases {
		got, ids := ResolveMentions(c.in, people)
		if got != c.want || !reflect.DeepEqual(ids, c.ids) {
			t.Errorf("ResolveMentions(%q) = %q %v, want %q %v", c.in, got, ids, c.want, c.ids)
		}
	}
}

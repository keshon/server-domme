package knowledge

import (
	"testing"

	st "github.com/keshon/server-domme/internal/storage"
)

func TestRetrieveRanksTitleHigher(t *testing.T) {
	docs := []st.KnowledgeDoc{
		{Slug: "a", Title: "Spoiler rules", Body: "unrelated text here"},
		{Slug: "b", Title: "Welcome", Body: "spoiler policy details spoiler"},
	}
	top := Retrieve(docs, "spoiler rules", 5)
	if len(top) != 2 || top[0].Slug != "a" {
		t.Fatalf("got %+v", top)
	}
}

func TestRetrieveNoMatch(t *testing.T) {
	docs := []st.KnowledgeDoc{{Slug: "a", Title: "Rules", Body: "be kind"}}
	if top := Retrieve(docs, "xyzzy plugh", 5); len(top) != 0 {
		t.Fatalf("got %+v", top)
	}
}

func TestRetrieveShortTokensIgnored(t *testing.T) {
	docs := []st.KnowledgeDoc{{Slug: "a", Title: "Rules", Body: "be kind"}}
	if top := Retrieve(docs, "a an of", 5); len(top) != 0 {
		t.Fatalf("got %+v", top)
	}
}

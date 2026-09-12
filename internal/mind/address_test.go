package mind

import "testing"

var herNames = []string{"Domme", "ServerDomme", "Server Domme"}

func TestClassifyAddressSpeakingToHer(t *testing.T) {
	for _, text := range []string{
		"domme, what do you reckon",
		"Domme: look at this",
		"domme - you around?",
		"so domme what do you think about the rules",
		"is domme going to answer?",
		"домме, ты тут?",
	} {
		if got := ClassifyAddress(text, herNames); got != AddressedToHer {
			t.Errorf("ClassifyAddress(%q) = about, want addressed", text)
		}
	}
}

func TestClassifyAddressTalkingAboutHer(t *testing.T) {
	for _, text := range []string{
		"domme would hate this",
		"i swear domme has been quiet all week",
		"server domme never answers me",
		"she said something about it, domme i mean",
		"домме всегда молчит",
	} {
		if got := ClassifyAddress(text, herNames); got != AddressedToHer {
			continue
		}
		t.Errorf("ClassifyAddress(%q) = addressed, want about", text)
	}
}

// "domme would know, wouldn't you say" is about her despite the "you", which
// is why the verb check runs before the pronoun check.
func TestClassifyAddressPrefersTheVerbOverAStrayPronoun(t *testing.T) {
	got := ClassifyAddress("domme would know, wouldn't you say", herNames)
	if got != AboutHer {
		t.Error("a third-person verb after the name should outrank a loose 'you'")
	}
}

// Overhearing is the commoner case and silence is the cheaper mistake.
func TestClassifyAddressResolvesAmbiguityToOverhearing(t *testing.T) {
	for _, text := range []string{
		"domme",
		"anyway domme lol",
		"",
	} {
		if got := ClassifyAddress(text, herNames); got != AboutHer {
			t.Errorf("ClassifyAddress(%q) = addressed, want about when unclear", text)
		}
	}
}

func TestClassifyAddressIgnoresNamesInsideWords(t *testing.T) {
	// "dommes" is not "domme"; nothing here addresses her.
	if got := ClassifyAddress("the dommes are all quiet", []string{"domme"}); got != AboutHer {
		t.Error("matched a name glued inside a longer word")
	}
}

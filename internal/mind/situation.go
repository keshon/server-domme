package mind

import "strings"

// Situation is what kind of moment she is answering: the thing her voice
// examples are chosen by. Eight examples drawn at random from the whole card
// teach a voice in general; three drawn from how she answers this kind of
// moment teach it for the moment in front of her, and cost less.
//
// The appraisal names the situation of a message — what a message is, is a
// reading, and readings are the model's. The code names it only where it
// already knows: she is starting something herself, or about to go. The
// code then only files the examples; it never tells the voice what the
// situation is. See docs/persona-v3.md, G.
type Situation string

const (
	SituationGreeting   Situation = "greeting"
	SituationQuestion   Situation = "question"
	SituationRequest    Situation = "request"
	SituationSharing    Situation = "sharing"
	SituationJab        Situation = "jab"
	SituationCompliment Situation = "compliment"
	SituationApology    Situation = "apology"
	SituationPushing    Situation = "pushing"
	SituationBanter     Situation = "banter"
	SituationGesture    Situation = "gesture"
	// Named by the code from the trigger, never by the appraisal.
	SituationStarting Situation = "starting"
	SituationLeaving  Situation = "leaving"
)

// appraisedSituations are the ones the appraisal may name, in the order the
// prompt lists them.
var appraisedSituations = []Situation{
	SituationGreeting, SituationQuestion, SituationRequest, SituationSharing, SituationJab,
	SituationCompliment, SituationApology, SituationPushing, SituationBanter, SituationGesture,
}

// Situations are every situation an example can be filed under.
func Situations() []Situation {
	return append(append([]Situation(nil), appraisedSituations...), SituationStarting, SituationLeaving)
}

// ParseSituation reads a situation the appraisal named, or a heading in the
// character file. Anything outside the list is "": a situation the model
// made up matches no examples, and the sample falls back to a random one.
func ParseSituation(s string) Situation {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, known := range Situations() {
		if string(known) == s {
			return known
		}
	}
	return ""
}

// situationShape is the appraisal's line naming the situation.
func situationShape() string {
	names := make([]string, len(appraisedSituations))
	for i, s := range appraisedSituations {
		names[i] = string(s)
	}
	return `  "situation": "what kind of message this is, one of: ` + strings.Join(names, ", ") +
		` — a gesture is something done to her; pushing is pressing past a no",` + "\n"
}

// situationOf is the situation her voice is answering: what the code knows
// from the trigger, otherwise what the appraisal named. Joining a
// conversation she overheard is answering what was said, so it goes by the
// appraisal.
func situationOf(s Scene, a Appraisal) Situation {
	switch {
	case s.Trigger == TriggerLeave:
		return SituationLeaving
	case Initiated(s.Trigger), s.Trigger == TriggerSight:
		return SituationStarting
	}
	return a.Situation
}

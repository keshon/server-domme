// Package mind builds what the bot says from what it knows.
//
// The organising idea, carried over from the cognitum experiment: the language
// model is a speech cortex, not a brain. It is handed an assembled picture of
// who the character is, where she is and who she is talking to, and its only
// job is to put that into words. Every decision about whether there is
// anything to say belongs on this side of the boundary, in ordinary Go that
// can be read, tested and stepped through.
//
// That split is also why personality here is not a set of numbers rendered
// into instructions. An earlier version of this package turned trait floats
// into lines like "Speak warmly and welcoming", which is how a character
// becomes a generic assistant: prose describing a voice is a much weaker
// signal to a language model than examples of that voice. So the character is
// authored text plus example exchanges (see Character), and numbers are kept
// for deciding when she speaks rather than how she sounds.
package mind

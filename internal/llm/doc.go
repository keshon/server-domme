// Package llm is the stateless tool backend: one OpenAI-compatible call per
// user request, no memory, no autonomy, no persona.
//
// Calls go through a shared Pool: the explicit endpoint first, then
// LLM_BACKENDS extras, then free public relays (g4f.space catalogue picks,
// pollinations). The pool fails over between them and rests backends that
// keep failing, so one dead relay is a slow call rather than a dead feature.
//
// The mention router calls ParseIntent, slash helpers call Complete. Both
// fail closed: disabled, unconfigured, timeout or bad JSON means the caller
// falls back to deterministic behavior and tells the user how.
package llm

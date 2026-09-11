package mind

import (
	"fmt"
	"sync"
	"testing"
)

func TestEncountersReportsAFirstApproach(t *testing.T) {
	e := NewEncounters()

	first, ignored := e.Approach("g:c:u")
	if !first {
		t.Error("first approach not reported as first")
	}
	if ignored {
		t.Error("a stranger cannot have been ignored before")
	}

	e.Record("g:c:u", OutcomeSpeak)

	first, _ = e.Approach("g:c:u")
	if first {
		t.Error("second approach still reported as first")
	}
}

func TestEncountersRemembersAnIgnore(t *testing.T) {
	e := NewEncounters()

	e.Record("g:c:u", OutcomeIgnore)
	if _, ignored := e.Approach("g:c:u"); !ignored {
		t.Error("the ignore was not remembered")
	}

	// Answering clears the debt, so two ignores in a row need two consecutive
	// ignore decisions rather than one ever-sticky flag.
	e.Record("g:c:u", OutcomeSpeak)
	if _, ignored := e.Approach("g:c:u"); ignored {
		t.Error("the ignore survived an answer")
	}
}

// Being ignored in one channel should not force a reply in another.
func TestEncountersKeepsKeysApart(t *testing.T) {
	e := NewEncounters()
	e.Record("g:c1:u", OutcomeIgnore)

	if _, ignored := e.Approach("g:c2:u"); ignored {
		t.Error("an ignore leaked across channels")
	}
}

func TestEncountersIsBounded(t *testing.T) {
	e := NewEncounters()
	for i := 0; i < maxEncounters+10; i++ {
		e.Record(fmt.Sprintf("g:c:u%d", i), OutcomeSpeak)
	}
	if e.Len() > maxEncounters {
		t.Errorf("holding %d encounters, want at most %d", e.Len(), maxEncounters)
	}
}

func TestEncountersAreSafeUnderConcurrentUse(t *testing.T) {
	e := NewEncounters()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("g:c:u%d", i%4)
			e.Approach(key)
			e.Record(key, OutcomeSpeak)
		}(i)
	}
	wg.Wait()
}

// bodysim runs the persona's body alone — no model, no Discord — over a
// synthetic stretch of days, and prints what she did: when she slept and
// woke, when she was online and away, and her sleep pressure and energy
// through the day. It is where the body's constants are tuned, cheaply,
// before anyone has to wait a week to see them. See docs/persona-v3.md, B.
//
//	go run ./cmd/bodysim
//	go run ./cmd/bodysim -days 14 -seed 7 -tz Europe/Moscow
//	go run ./cmd/bodysim -busy 3 -every 15m
//
// -busy makes one evening a long conversation — engaged from 20:00 to 01:30,
// a moment every few minutes — to show a late night carried into the next
// morning. -every prints the state at that interval as well as every change.
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/body"
)

// Busy evening: when it runs, how often she handles a moment, and what each
// costs her, as the chat service drains it.
const (
	busyFrom    = 20 * time.Hour
	busyUntil   = 25*time.Hour + 30*time.Minute
	busyEvery   = 4 * time.Minute
	busyDrain   = 0.03
	tickEvery   = time.Minute
	defaultDays = 7
)

func main() {
	days := flag.Int("days", defaultDays, "days to simulate")
	seed := flag.Uint64("seed", 1, "seed for life's interruptions")
	tz := flag.String("tz", "UTC", "the community's timezone, as CHAT_TIMEZONE")
	busy := flag.Int("busy", 0, "make evening N (from 1) a long conversation; 0 for none")
	every := flag.Duration("every", 0, "also print the state at this interval; 0 for changes only")
	flag.Parse()

	loc, err := time.LoadLocation(*tz)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bodysim:", err)
		os.Exit(1)
	}
	r := rand.New(rand.NewPCG(*seed, *seed))
	start := time.Date(2026, 9, 21, 12, 0, 0, 0, loc)
	b := body.New(loc, r.Float64, start)

	var busyStart time.Time
	if *busy > 0 {
		busyStart = start.Truncate(24*time.Hour).AddDate(0, 0, *busy-1)
	}
	online := map[string]time.Duration{}
	stretches := map[string]int{}
	var lastPrint, lastMoment time.Time
	end := start.AddDate(0, 0, *days)

	fmt.Printf("%-16s %-8s %5s %5s %6s  %s\n", "time", "presence", "S", "B", "sleepy", "change")
	for now := start; now.Before(end); now = now.Add(tickEvery) {
		engaged := false
		if !busyStart.IsZero() {
			since := now.Sub(busyStart)
			engaged = since >= busyFrom && since < busyUntil
		}
		if engaged && b.State().Presence == body.Online && now.Sub(lastMoment) >= busyEvery {
			b.Drain(busyDrain)
			lastMoment = now
		}
		for _, e := range b.Advance(now, engaged) {
			if e.To == body.Online {
				stretches[e.At.Format("2006-01-02")]++
			}
			printState(b, e.At, fmt.Sprintf("%s → %s (%s)", e.From, e.To, e.Why))
			lastPrint = now
		}
		if b.State().Presence == body.Online {
			online[now.Format("2006-01-02")] += tickEvery
		}
		if *every > 0 && now.Sub(lastPrint) >= *every {
			note := ""
			if engaged {
				note = "in a conversation"
			}
			printState(b, now, note)
			lastPrint = now
		}
	}

	fmt.Println()
	fmt.Printf("%-10s %9s %9s\n", "day", "online", "stretches")
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		fmt.Printf("%-10s %9s %9d\n", key, strings.TrimSuffix(online[key].Round(time.Minute).String(), "0s"), stretches[key])
	}
}

func printState(b *body.Body, at time.Time, note string) {
	st := b.State()
	fmt.Printf("%-16s %-8s %5.2f %5.2f %6.2f  %s\n",
		at.Format("Mon 02 15:04"), st.Presence, st.S, st.B, b.Sleepiness(at), note)
}

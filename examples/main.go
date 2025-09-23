package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	breaker "github.com/afk-ankit/sparkgap"
)

func accounts(s string, broke bool) (string, error) {
	if broke {
		return "", fmt.Errorf("service broke")
	}
	return fmt.Sprintf("Hi %s", s), nil
}

func main() {
	br := breaker.InitBreaker[string]("accounts", &breaker.BreakerConfig{})

	app := tview.NewApplication()

	// Logs (left)
	logs := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { app.Draw() })
	logs.SetBorder(true).SetTitle(" Logs ")

	// Breaker state (right)
	state := tview.NewTextView().
		SetDynamicColors(false).
		SetChangedFunc(func() { app.Draw() })
	state.SetBorder(true).SetTitle(" Circuit Breaker ")

	// Layout
	flex := tview.NewFlex().
		AddItem(logs, 0, 3, false).
		AddItem(state, 48, 0, false)

	// Update right every second
	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for range t.C {
			s := capture(func() { br.LogStateString() })
			app.QueueUpdateDraw(func() {
				state.SetText(s)
			})
		}
	}()

	// Simulate service flaps
	broke := false
	go func() {
		events := []struct {
			msg   string
			delay time.Duration
			val   bool
		}{
			{"service B is down", 5 * time.Second, true},
			{"service B is up", 15 * time.Second, false},
			{"service B is down", 5 * time.Second, true},
			{"service B is up", 10 * time.Second, false},
		}
		for _, e := range events {
			time.Sleep(e.delay)
			broke = e.val
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(logs, "%s\n", e.msg)
			})
		}
	}()

	// Workload loop
	go func() {
		for range 100 {
			time.Sleep(1 * time.Second)
			val, err := br.Execute(func() (string, error) { return accounts("ankit", broke) })
			app.QueueUpdateDraw(func() {
				if err != nil {
					fmt.Fprintf(logs, "[red]%s[-]\n", err.Error())
				} else {
					fmt.Fprintf(logs, "%s\n", val)
				}
			})
		}
	}()

	// Quit keys
	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Rune() == 'q' || ev.Key() == tcell.KeyESC {
			app.Stop()
			return nil
		}
		return ev
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

// capture stdout of fn into a string (used to render LogState)
func capture(fn func()) string {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()
	return buf.String()
}

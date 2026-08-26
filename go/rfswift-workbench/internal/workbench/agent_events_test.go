package workbench

import (
	"errors"
	"testing"
)

func TestAgentTerminalEventsStreamCommandOutputAndExit(t *testing.T) {
	a := testApp(t, &fakeEngine{})
	w, err := newAgentEventWriter(a.store, a.ws, "rfid", "printf hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hel")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("lo\n")); err != nil {
		t.Fatal(err)
	}
	w.finish(errors.New("exit status 7"))

	got, err := a.ReadAgentTerminalEvents("rfid", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 4 || got.Cursor == 0 {
		t.Fatalf("events = %#v, cursor = %d", got.Events, got.Cursor)
	}
	if got.Events[0].Type != "start" || got.Events[0].Command != "printf hello" {
		t.Fatalf("start event = %#v", got.Events[0])
	}
	if got.Events[1].Data+got.Events[2].Data != "hello\n" {
		t.Fatalf("streamed output = %q", got.Events[1].Data+got.Events[2].Data)
	}
	if got.Events[3].Type != "exit" || got.Events[3].Error != "exit status 7" {
		t.Fatalf("exit event = %#v", got.Events[3])
	}

	next, err := a.ReadAgentTerminalEvents("rfid", got.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Events) != 0 || next.Cursor != got.Cursor {
		t.Fatalf("cursor replayed events: %#v", next)
	}
}

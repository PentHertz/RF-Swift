package rfutils

import (
	"testing"
	"time"
)

// Captured with `wevtutil qe Microsoft-Windows-TerminalServices-RDPClient/Operational /f:xml`
// on a Windows 11 host (WSL 2.7.12, WSLg 1.0.73) while SDR++ showed only a
// taskbar icon: the client connected (1025), then its graphics decoder
// failed (1033 RdpGfx*, 226, 1404) and it dropped the connection (1033 slint).
const sampleWSLgClientEvents = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-TerminalServices-ClientActiveXCore' Guid='{28aa95bb-d444-4719-a36f-40462168127e}'/><EventID>1025</EventID><Version>0</Version><Level>4</Level><Task>101</Task><Opcode>10</Opcode><Keywords>0x4000000000000000</Keywords><TimeCreated SystemTime='2026-09-03T17:56:40.1000000Z'/><EventRecordID>107001</EventRecordID><Correlation ActivityID='{ca46d06d-896c-4336-bd77-36a105f90000}'/><Execution ProcessID='3244' ThreadID='3456'/><Channel>Microsoft-Windows-TerminalServices-RDPClient/Operational</Channel><Computer>DESKTOP</Computer><Security UserID='S-1-5-21-1'/></System><EventData></EventData></Event>
<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-TerminalServices-ClientActiveXCore' Guid='{28aa95bb-d444-4719-a36f-40462168127e}'/><EventID>1033</EventID><Version>0</Version><Level>2</Level><Task>100</Task><Opcode>48</Opcode><Keywords>0x4000000000000000</Keywords><TimeCreated SystemTime='2026-09-03T17:56:47.2000000Z'/><EventRecordID>107002</EventRecordID><Execution ProcessID='3244' ThreadID='3456'/><Channel>Microsoft-Windows-TerminalServices-RDPClient/Operational</Channel><Computer>DESKTOP</Computer><Security UserID='S-1-5-21-1'/></System><EventData><Data Name='Name'>RdpGfxProtocolClientDecoder</Data><Data Name='CustomLevel'>WireDecoderError_DecodeCreateSurfaceCreateError(71)</Data><Data Name='Value'>0x80004003</Data></EventData></Event>
<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-TerminalServices-ClientActiveXCore' Guid='{28aa95bb-d444-4719-a36f-40462168127e}'/><EventID>226</EventID><Version>0</Version><Level>3</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x4000000000000000</Keywords><TimeCreated SystemTime='2026-09-03T17:56:47.3000000Z'/><EventRecordID>107003</EventRecordID><Execution ProcessID='3244' ThreadID='3456'/><Channel>Microsoft-Windows-TerminalServices-RDPClient/Operational</Channel><Computer>DESKTOP</Computer><Security UserID='S-1-5-21-1'/></System><EventData><Data>GfxStateDecodingRdpGfxPdu</Data><Data>GfxStateError</Data><Data>GfxEventDecodingCreateSurfacePduFailed</Data><Data>0x80004003</Data></EventData></Event>
<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Microsoft-Windows-TerminalServices-ClientActiveXCore' Guid='{28aa95bb-d444-4719-a36f-40462168127e}'/><EventID>1033</EventID><Version>0</Version><Level>2</Level><Task>100</Task><Opcode>48</Opcode><Keywords>0x4000000000000000</Keywords><TimeCreated SystemTime='2026-09-03T17:58:50.0000000Z'/><EventRecordID>107004</EventRecordID><Execution ProcessID='3244' ThreadID='3456'/><Channel>Microsoft-Windows-TerminalServices-RDPClient/Operational</Channel><Computer>DESKTOP</Computer><Security UserID='S-1-5-21-1'/></System><EventData><Data Name='Name'>slint</Data><Data Name='CustomLevel'>SL::OnDisconnected</Data><Data Name='Value'>0x904</Data></EventData></Event>
`

func TestParseWSLgClientEvents(t *testing.T) {
	events := parseWSLgClientEvents("\ufeff" + sampleWSLgClientEvents) // wevtutil output may start with a BOM
	if len(events) != 4 {
		t.Fatalf("parsed %d events, want 4", len(events))
	}
	connected := events[0]
	if connected.ID != 1025 || connected.ProcessID != 3244 {
		t.Errorf("first event = %+v, want id 1025 of pid 3244", connected)
	}
	if want := time.Date(2026, 9, 3, 17, 56, 40, 100000000, time.UTC); !connected.Time.Equal(want) {
		t.Errorf("first event time = %v, want %v", connected.Time, want)
	}
	gfx := events[1]
	if gfx.Component != "RdpGfxProtocolClientDecoder" || !gfx.gfxError() {
		t.Errorf("second event = %+v, want an RdpGfx graphics error", gfx)
	}
	if got, want := gfx.describe(), "RdpGfxProtocolClientDecoder WireDecoderError_DecodeCreateSurfaceCreateError(71) 0x80004003"; got != want {
		t.Errorf("describe() = %q, want %q", got, want)
	}
	if !events[2].gfxError() {
		t.Errorf("event 226 must count as a graphics error")
	}
	if events[3].gfxError() {
		t.Errorf("a 1033 disconnect (slint) must not count as a graphics error")
	}
	if got := parseWSLgClientEvents("not xml at all"); got != nil {
		t.Errorf("malformed input parsed as %v, want nil", got)
	}
	if got := parseWSLgClientEvents(""); got != nil {
		t.Errorf("empty input parsed as %v, want nil", got)
	}
}

func TestWSLgDisplayStateFrom(t *testing.T) {
	events := parseWSLgClientEvents(sampleWSLgClientEvents)

	st := wslgDisplayStateFrom(true, 3244, events)
	if !st.Degraded {
		t.Fatalf("errors after the last connection must read as degraded: %+v", st)
	}
	if st.LastGfxError.Before(st.LastConnected) || st.LastGfxErrorText == "" {
		t.Errorf("state = %+v, want the newest graphics error after the connection", st)
	}
	if st.Summary() == "" || st.Summary() == "not running (starts with the first GUI window)" {
		t.Errorf("summary = %q", st.Summary())
	}

	// A reconnection after the errors clears the condition.
	recovered := append(events, wslgClientEvent{ID: wslgClientConnectedEvent, ProcessID: 3244, Time: time.Date(2026, 9, 3, 18, 0, 25, 0, time.UTC)})
	if st := wslgDisplayStateFrom(true, 3244, recovered); st.Degraded {
		t.Errorf("a connection after the errors must not read as degraded: %+v", st)
	}

	// Events of another Remote Desktop client process are not this client's.
	if st := wslgDisplayStateFrom(true, 9999, events); st.Degraded || !st.LastGfxError.IsZero() {
		t.Errorf("events of pid 3244 were attributed to pid 9999: %+v", st)
	}

	// No client: nothing to say, and never degraded.
	if st := wslgDisplayStateFrom(false, 0, events); st.Degraded || st.ClientRunning {
		t.Errorf("no client must be neither running nor degraded: %+v", st)
	}
	if got := wslgDisplayStateFrom(false, 0, nil).Summary(); got != "not running (starts with the first GUI window)" {
		t.Errorf("summary without a client = %q", got)
	}

	// A running client without any record is simply running.
	if st := wslgDisplayStateFrom(true, 42, nil); st.Degraded || !st.ClientRunning || st.ClientPID != 42 {
		t.Errorf("running client without events = %+v", st)
	}
}

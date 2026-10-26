package dock

import "testing"

func TestConfigStageLineRoundTrip(t *testing.T) {
	stage, percent, ok := ParseConfigStageLine(ConfigStageLine("Restarting the Docker service\n(containers stop)", 55))
	if !ok || percent != 55 || stage != "Restarting the Docker service (containers stop)" {
		t.Fatalf("round trip = %q %d %v", stage, percent, ok)
	}
	for _, line := range []string{"", "[i] Container engine: Docker", "rfswift-config-stage:", "rfswift-config-stage:x:Stopping", "rfswift-config-stage:150:Stopping", "rfswift-config-stage:20:"} {
		if _, _, ok := ParseConfigStageLine(line); ok {
			t.Errorf("%q parsed as a stage", line)
		}
	}
}

func TestConfigStageReporter(t *testing.T) {
	var got []string
	SetConfigStageReporter(func(stage string, percent int) { got = append(got, stage) })
	reportConfigStage("Stopping the container", 15)
	SetConfigStageReporter(nil)
	reportConfigStage("Starting the container", 85) // no reporter: ignored
	if len(got) != 1 || got[0] != "Stopping the container" {
		t.Fatalf("reported %v", got)
	}
}

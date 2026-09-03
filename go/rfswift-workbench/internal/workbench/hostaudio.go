/* This code is part of RF Swift by @Penthertz
*  Author(s): Sébastien Dudek (@FlUxIuS)
 */

package workbench

import (
	"errors"
	"fmt"
	"log"
	"runtime"

	rfdock "penthertz/rfswift/dock"
	"penthertz/rfswift/rfutils"
)

// Host audio for container missions (Docker, Podman, Lima). Inside the
// container PULSE_SERVER points at the host (config [audio] pulse_server,
// default tcp:localhost:34567); on the host that target only exists once the
// audio server's module-native-protocol-tcp is loaded. The CLI loads it on
// every `rfswift run`; these bindings do the same for the Workbench (create
// and start), show the state in the engine doctor, and let a target's context
// menu enable or disable it. Windows needs none of this: containers use the
// WSLg PulseAudio socket. Nix environments run natively and play sound
// directly.

// hostAudioApplies reports whether this App can act on the host audio server
// at all (local engine, not Windows).
func (a *App) hostAudioApplies() bool {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return false
	}
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
}

// HostAudioStatus reports the host audio server state for the GUI. Never
// starts or changes anything.
func (a *App) HostAudioStatus() rfutils.HostAudioStatus {
	if _, ok := a.eng.(*LocalEngine); !ok {
		return rfutils.HostAudioStatus{Server: rfdock.PulseServer(), Detail: "host audio is managed on the remote host"}
	}
	return rfutils.GetHostAudioStatus(rfdock.PulseServer())
}

// HostAudioEnable loads the audio server's TCP module on the host (idempotent)
// and returns the resulting state.
func (a *App) HostAudioEnable() (rfutils.HostAudioStatus, error) {
	if !a.hostAudioApplies() {
		return a.HostAudioStatus(), errors.New("host audio is only managed for local Linux and macOS engines")
	}
	if err := rfutils.SetPulseCTL(rfdock.PulseServer()); err != nil {
		return a.HostAudioStatus(), err
	}
	st := a.HostAudioStatus()
	if st.Detail != "" {
		st.Detail = "Host audio server enabled: " + st.Detail
	}
	return st, nil
}

// HostAudioDisable unloads the audio server's TCP module on the host. Every
// running container loses its audio path until it is enabled again.
func (a *App) HostAudioDisable() (rfutils.HostAudioStatus, error) {
	if !a.hostAudioApplies() {
		return a.HostAudioStatus(), errors.New("host audio is only managed for local Linux and macOS engines")
	}
	if err := rfutils.UnloadPulseCTL(); err != nil {
		return a.HostAudioStatus(), err
	}
	st := a.HostAudioStatus()
	st.Detail = fmt.Sprintf("Host audio server disabled: %s unloaded (containers get no sound until it is enabled again)", rfutils.PulseTCPModule)
	return st, nil
}

// SetMissionHostAudio enables or disables the host audio server now and
// remembers the choice for the mission's next starts (HostAudioOff in
// mission.json). The module is host-wide: disabling it also silences the
// other running containers.
func (a *App) SetMissionHostAudio(id string, enable bool) (rfutils.HostAudioStatus, error) {
	if err := a.requireMission(id); err != nil {
		return rfutils.HostAudioStatus{}, err
	}
	if isNixEnv(id) {
		return rfutils.HostAudioStatus{}, errors.New("Nix environments play sound natively; there is no host audio server to manage")
	}
	if !a.hostAudioApplies() {
		return a.HostAudioStatus(), errors.New("host audio is only managed for local Linux and macOS engines")
	}
	if err := a.setMissionHostAudioOff(id, !enable); err != nil {
		return a.HostAudioStatus(), err
	}
	if enable {
		return a.HostAudioEnable()
	}
	return a.HostAudioDisable()
}

// setMissionHostAudioOff stores the per-mission preference without touching
// the other saved fields (title, notes, audit counters).
func (a *App) setMissionHostAudioOff(id string, off bool) error {
	saved, err := a.store.ListMissions(a.ws)
	if err != nil {
		return err
	}
	for _, m := range saved {
		if m.ID == id {
			m.HostAudioOff = off
			return a.store.SaveMission(a.ws, m)
		}
	}
	return fmt.Errorf("mission %q is not recorded in this project", id)
}

// ensureMissionHostAudio loads the host audio server's TCP module before a
// container mission starts, unless the mission opted out (HostAudioOff).
// Best effort: a missing pactl or audio server never blocks the start; the
// outcome is logged, and the engine doctor shows the state.
func (a *App) ensureMissionHostAudio(id string) {
	if !a.hostAudioApplies() || isNixEnv(id) {
		return
	}
	if saved, err := a.store.ListMissions(a.ws); err == nil {
		for _, m := range saved {
			if m.ID == id && m.HostAudioOff {
				return
			}
		}
	}
	if err := rfutils.SetPulseCTL(rfdock.PulseServer()); err != nil {
		log.Printf("rfswift-workbench: host audio server not enabled for %s: %v", id, err)
	}
}

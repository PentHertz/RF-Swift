package nix

import rfutils "penthertz/rfswift/rfutils"

// ParseProbeForTest builds a backend status for tests without a distribution.
func ParseProbeForTest(distro string, nix, rfswift bool) rfutils.WSLNixStatus {
	st := rfutils.WSLNixStatus{Distro: distro, User: "u", Home: "/home/u"}
	if nix {
		st.NixVersion = "nix (Nix) 2.31.2"
	}
	if rfswift {
		st.RFSwiftVersion = "4.0.1-dev"
	}
	return st
}

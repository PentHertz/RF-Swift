//go:build windows

package cli

type deviceAccessInfo struct {
	Kind        string
	Accessible  bool
	Group       string
	GroupAccess bool
	OwnerRoot   bool
}

func deviceAccess(string) deviceAccessInfo { return deviceAccessInfo{Kind: "device", Accessible: true} }

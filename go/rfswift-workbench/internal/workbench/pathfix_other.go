//go:build !windows

package workbench

func platformPathCandidates(home string) []string { return nil }

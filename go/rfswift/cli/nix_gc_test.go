package cli

import "testing"

func TestParseSize(t *testing.T) {
	ok := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"1024", 1024},
		{"1K", 1024},
		{"500M", 500 * 1024 * 1024},
		{"5G", 5 * 1024 * 1024 * 1024},
		{"2T", 2 * 1024 * 1024 * 1024 * 1024},
		{"5GB", 5 * 1024 * 1024 * 1024},
		{"5GiB", 5 * 1024 * 1024 * 1024},
		{"5gib", 5 * 1024 * 1024 * 1024},
		{" 500m ", 500 * 1024 * 1024},
	}
	for _, c := range ok {
		got, err := parseSize(c.in)
		if err != nil {
			t.Errorf("parseSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	bad := []string{"abc", "5X", "-1", "5 G B", "1.5G"}
	for _, in := range bad {
		if _, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q) expected an error, got nil", in)
		}
	}
}

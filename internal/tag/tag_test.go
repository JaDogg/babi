package tag

import "testing"

func TestBumpVersion(t *testing.T) {
	cases := []struct {
		tag  string
		bump string
		want string
	}{
		{"v1.2.3", "patch", "v1.2.4"},
		{"v1.2.3", "minor", "v1.3.0"},
		{"v1.2.3", "major", "v2.0.0"},
		{"v0.0.1", "patch", "v0.0.2"},
		{"v0.9.9", "minor", "v0.10.0"},
		{"v0.9.9", "major", "v1.0.0"},
		{"v1.0.0", "patch", "v1.0.1"},
		// pre-release suffix stripped before bump
		{"v1.2.3-rc1", "patch", "v1.2.4"},
		// no "v" prefix preserved
		{"1.2.3", "patch", "1.2.4"},
	}
	for _, tc := range cases {
		got, err := bumpVersion(tc.tag, tc.bump)
		if err != nil {
			t.Errorf("bumpVersion(%q, %q) error: %v", tc.tag, tc.bump, err)
			continue
		}
		if got != tc.want {
			t.Errorf("bumpVersion(%q, %q) = %q, want %q", tc.tag, tc.bump, got, tc.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		input         string
		major, minor, patch int
		prefix        string
	}{
		{"v1.2.3", 1, 2, 3, "v"},
		{"v0.0.1", 0, 0, 1, "v"},
		{"1.2.3", 1, 2, 3, ""},
		{"v10.20.30", 10, 20, 30, "v"},
	}
	for _, tc := range cases {
		v, err := parseSemver(tc.input)
		if err != nil {
			t.Errorf("parseSemver(%q) error: %v", tc.input, err)
			continue
		}
		if v.major != tc.major || v.minor != tc.minor || v.patch != tc.patch || v.prefix != tc.prefix {
			t.Errorf("parseSemver(%q) = {%d %d %d %q}, want {%d %d %d %q}",
				tc.input, v.major, v.minor, v.patch, v.prefix,
				tc.major, tc.minor, tc.patch, tc.prefix)
		}
	}
}

func TestParseSemverErrors(t *testing.T) {
	bad := []string{"notaversion", "v1.2", "v1.2.x", ""}
	for _, s := range bad {
		if _, err := parseSemver(s); err == nil {
			t.Errorf("parseSemver(%q) expected error, got nil", s)
		}
	}
}

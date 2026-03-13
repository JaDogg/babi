package cf

import (
	"strings"
	"testing"
)

func script(delim, format string, maxsplit int, extended bool, setup, foreach, filter, filterAfter, run string, skip, limit int, strip bool, join string) string {
	return buildScript(delim, format, maxsplit, extended, setup, foreach, filter, filterAfter, run, skip, limit, strip, join)
}

func defaults(delim, format string) string {
	return script(delim, format, 0, false, "", "", "", "", "", 0, 0, false, "")
}

func TestBasic(t *testing.T) {
	s := defaults(",", "{part[0]}: {part[1]}")
	assertContains(t, s, `part = line.split(""",""")`)
	assertContains(t, s, `print(f"""{part[0]}: {part[1]}""")`)
	assertNotContains(t, s, "import re")
	assertNotContains(t, s, "import os")
}

func TestSkip(t *testing.T) {
	s := script(",", "{part[0]}", 0, false, "", "", "", "", "", 2, 0, false, "")
	assertContains(t, s, "if n < 2:\n        continue")
}

func TestSkipZeroOmitted(t *testing.T) {
	s := defaults(",", "{part[0]}")
	assertNotContains(t, s, "if n <")
}

func TestStrip(t *testing.T) {
	s := script(",", "{part[0]}", 0, false, "", "", "", "", "", 0, 0, true, "")
	assertContains(t, s, "part = [f.strip() for f in part]")
	// strip must come after the split line
	splitIdx := strings.Index(s, "part = line.split")
	stripIdx := strings.Index(s, "part = [f.strip()")
	if stripIdx <= splitIdx {
		t.Errorf("strip must appear after split; splitIdx=%d stripIdx=%d", splitIdx, stripIdx)
	}
}

func TestStripFalseOmitted(t *testing.T) {
	s := defaults(",", "{part[0]}")
	assertNotContains(t, s, "f.strip()")
}

func TestJoin(t *testing.T) {
	s := script(",", "{part[0]}", 0, false, "", "", "", "", "", 0, 0, false, ", ")
	assertContains(t, s, "_results = []")
	assertContains(t, s, `_results.append(f"""{part[0]}""")`)
	assertContains(t, s, `print(""", """.join(_results))`)
	assertNotContains(t, s, "print(f\"\"\"")
}

func TestJoinNoBufferWithRun(t *testing.T) {
	// --run takes priority; --join should not add buffering
	s := script(",", "{part[0]}", 0, false, "", "", "", "", "echo {output}", 0, 0, false, ", ")
	assertNotContains(t, s, "_results")
	assertContains(t, s, "os.system")
}

func TestFilter(t *testing.T) {
	s := script(" ", "{part[0]}", 0, false, "", "", `^ERROR`, "", "", 0, 0, false, "")
	assertContains(t, s, "import re")
	assertContains(t, s, `if not re.search("""^ERROR""", line):`)
	assertContains(t, s, "continue")
	// filter must come before split
	filterIdx := strings.Index(s, "re.search")
	splitIdx := strings.Index(s, "part = line.split")
	if filterIdx >= splitIdx {
		t.Errorf("filter must appear before split; filterIdx=%d splitIdx=%d", filterIdx, splitIdx)
	}
}

func TestFilterAfter(t *testing.T) {
	s := script(",", "{part[0]}", 0, false, "", "", "", "len(part) > 2", "", 0, 0, false, "")
	assertContains(t, s, "if not (len(part) > 2):\n        continue")
	// filter-after must come after split and strip
	splitIdx := strings.Index(s, "part = line.split")
	filterAfterIdx := strings.Index(s, "if not (len(part)")
	printIdx := strings.Index(s, "print(f\"\"\"")
	if filterAfterIdx <= splitIdx {
		t.Errorf("filter-after must appear after split; splitIdx=%d filterAfterIdx=%d", splitIdx, filterAfterIdx)
	}
	if filterAfterIdx >= printIdx {
		t.Errorf("filter-after must appear before format/print; filterAfterIdx=%d printIdx=%d", filterAfterIdx, printIdx)
	}
}

func TestFilterAfterAfterStrip(t *testing.T) {
	s := script(",", "{part[0]}", 0, false, "", "", "", "part[0] != ''", "", 0, 0, true, "")
	stripIdx := strings.Index(s, "part = [f.strip()")
	filterAfterIdx := strings.Index(s, "if not (part[0]")
	if filterAfterIdx <= stripIdx {
		t.Errorf("filter-after must appear after strip; stripIdx=%d filterAfterIdx=%d", stripIdx, filterAfterIdx)
	}
}

func TestFilterAfterOmittedWhenEmpty(t *testing.T) {
	s := defaults(",", "{part[0]}")
	assertNotContains(t, s, "if not (")
}

func TestExtended(t *testing.T) {
	s := script(`\s+`, "{part[0]}", 0, true, "", "", "", "", "", 0, 0, false, "")
	assertContains(t, s, "import re")
	assertContains(t, s, `part = re.split("""\s+""", line)`)
}

func TestMaxsplit(t *testing.T) {
	s := script(" ", "{part[0]}", 3, false, "", "", "", "", "", 0, 0, false, "")
	assertContains(t, s, `part = line.split(""" """, 3)`)
}

func TestSetup(t *testing.T) {
	s := script(",", "{total:.2f}", 0, false, "total=0", "total+=float(part[0])", "", "", "", 0, 0, false, "")
	assertContains(t, s, "total=0\n")
	assertContains(t, s, "total+=float(part[0])")
	// setup must appear before the loop
	setupIdx := strings.Index(s, "total=0")
	loopIdx := strings.Index(s, "for n, line")
	if setupIdx >= loopIdx {
		t.Errorf("setup must appear before loop; setupIdx=%d loopIdx=%d", setupIdx, loopIdx)
	}
}

func TestRun(t *testing.T) {
	s := script(" ", "{part[0]}", 0, false, "", "", "", "", "echo {output}", 0, 0, false, "")
	assertContains(t, s, "import os")
	assertContains(t, s, `output = f"""{part[0]}"""`)
	assertContains(t, s, `os.system(f"""echo {output}""")`)
	assertNotContains(t, s, "print(f\"\"\"")
}

func TestLimit(t *testing.T) {
	s := script(" ", "{part[0]}", 0, false, "", "", "", "", "", 0, 0, false, "")
	assertNotContains(t, s, "_emitted")

	s = script(" ", "{part[0]}", 0, false, "", "", "", "", "", 0, 3, false, "")
	assertContains(t, s, "_emitted = 0")
	assertContains(t, s, "_emitted += 1")
	assertContains(t, s, "if _emitted >= 3:\n            break")
	// counter must be inside the try block, after the print
	printIdx := strings.Index(s, "print(f\"\"\"")
	breakIdx := strings.Index(s, "break")
	if breakIdx <= printIdx {
		t.Errorf("limit break must appear after print; printIdx=%d breakIdx=%d", printIdx, breakIdx)
	}
}

func TestSkipAndLimit(t *testing.T) {
	// skip=2 means ignore lines 0 and 1; limit=3 means emit at most 3 lines.
	// The two are independent: skip filters by input index, limit counts emitted output.
	s := script(",", "{part[0]}", 0, false, "", "", "", "", "", 2, 3, false, "")
	assertContains(t, s, "if n < 2:\n        continue")
	assertContains(t, s, "_emitted = 0")
	assertContains(t, s, "if _emitted >= 3:\n            break")
	// skip must come before split, limit counter must come after print
	skipIdx := strings.Index(s, "if n < 2")
	splitIdx := strings.Index(s, "part = line.split")
	printIdx := strings.Index(s, "print(f\"\"\"")
	breakIdx := strings.Index(s, "break")
	if skipIdx >= splitIdx {
		t.Errorf("skip must appear before split; skipIdx=%d splitIdx=%d", skipIdx, splitIdx)
	}
	if breakIdx <= printIdx {
		t.Errorf("limit break must appear after print; printIdx=%d breakIdx=%d", printIdx, breakIdx)
	}
}

func TestSkipAndStripAndJoinCombined(t *testing.T) {
	s := script(",", "{part[1]}", 0, false, "", "", "", "", "", 1, 0, true, " | ")
	assertContains(t, s, "if n < 1:")
	assertContains(t, s, "part = [f.strip() for f in part]")
	assertContains(t, s, "_results = []")
	assertContains(t, s, `print(""" | """.join(_results))`)
}

// helpers

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected script to contain %q\nscript:\n%s", sub, s)
	}
}

func assertNotContains(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("expected script NOT to contain %q\nscript:\n%s", sub, s)
	}
}

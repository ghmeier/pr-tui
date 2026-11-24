package pullrequests

import (
	"reflect"
	"strings"
	"testing"
)

func TestHighlightingKeepsEveryCharacterOfTheDiffLine(t *testing.T) {
	files := parseDiff(fixtureDiff)

	rendered := []string{}
	for _, line := range files[0].lines {
		rendered = append(rendered, strings.TrimRight(line.renderLines(80)[0], " "))
	}

	expected := []string{
		"index 111..222 100644", "--- a/first.py", "+++ b/first.py", "@@ -1,3 +1,4 @@",
		" unchanged", "-removed", "+added", "+added again",
	}
	if !reflect.DeepEqual(rendered, expected) {
		t.Fatalf("rendered = %q", rendered)
	}
	if files[0].lines[4].code == nil {
		t.Fatalf("a Python file should be tokenised for highlighting")
	}
}

func TestParseDiffCountsChangesOnlyAfterAFilesFirstHunk(t *testing.T) {
	files := parseDiff(fixtureDiff)

	first, renamed := files[0], files[1]

	if first.additions != 2 || first.deletions != 1 {
		t.Fatalf("first file counts +%d -%d", first.additions, first.deletions)
	}
	if renamed.path != "second/new_name.py" || renamed.additions != 1 || renamed.deletions != 1 {
		t.Fatalf("renamed file = %s +%d -%d", renamed.path, renamed.additions, renamed.deletions)
	}
}

// oneSidedFixtureDiff has an added file, a deleted file, and a modified one.
const oneSidedFixtureDiff = `diff --git a/created.py b/created.py
new file mode 100644
index 000..111
--- /dev/null
+++ b/created.py
@@ -0,0 +1,2 @@
+first
+second
diff --git a/deleted.py b/deleted.py
deleted file mode 100644
index 111..000
--- a/deleted.py
+++ /dev/null
@@ -1 +0,0 @@
-gone
diff --git a/modified.py b/modified.py
index 111..222 100644
--- a/modified.py
+++ b/modified.py
@@ -1 +1 @@
-before
+after
`

func TestParseDiffMarksAddedAndDeletedFilesAsOneSided(t *testing.T) {
	files := parseDiff(oneSidedFixtureDiff)

	created, deleted := files[0], files[1]

	if !created.oneSided || !deleted.oneSided {
		t.Fatalf("one-sided: created %v, deleted %v", created.oneSided, deleted.oneSided)
	}
}

func TestParseDiffDoesNotMarkAModifiedFileAsOneSided(t *testing.T) {
	modified := parseDiff(oneSidedFixtureDiff)[2]

	if modified.oneSided {
		t.Fatalf("%s should have two sides", modified.path)
	}
}

func TestSplitViewPutsRemovedLineBesideItsReplacement(t *testing.T) {
	file := parseDiff(fixtureDiff)[0]

	rows := []string{}
	for _, row := range file.splitRows[4:] {
		rows = append(rows, strings.Join(strings.Fields(strings.Join(file.renderSplitRow(row, 60, " │ "), "\n")), " "))
	}

	expected := []string{"1 unchanged │ 1 unchanged", "2 -removed │ 2 +added", "│ 3 +added again"}
	if !reflect.DeepEqual(rows, expected) {
		t.Fatalf("rows = %q", rows)
	}
}

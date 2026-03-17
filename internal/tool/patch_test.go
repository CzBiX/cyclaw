package tool

import (
	"testing"
)

func TestApplyUnifiedDiff_SingleHunkAdd(t *testing.T) {
	original := "line1\nline2\nline3\n"
	diff := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+inserted
 line2
 line3
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\ninserted\nline2\nline3\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_SingleHunkRemove(t *testing.T) {
	original := "line1\nline2\nline3\n"
	diff := `@@ -1,3 +1,2 @@
 line1
-line2
 line3
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline3\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_Replace(t *testing.T) {
	original := "aaa\nbbb\nccc\n"
	diff := `@@ -1,3 +1,3 @@
 aaa
-bbb
+BBB
 ccc
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "aaa\nBBB\nccc\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_MultipleHunks(t *testing.T) {
	original := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	diff := `@@ -1,3 +1,3 @@
 1
-2
+TWO
 3
@@ -8,3 +8,3 @@
 8
-9
+NINE
 10
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "1\nTWO\n3\n4\n5\n6\n7\n8\nNINE\n10\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_ContextMismatch(t *testing.T) {
	original := "aaa\nbbb\nccc\n"
	diff := `@@ -1,3 +1,3 @@
 aaa
-xxx
+yyy
 ccc
`
	_, err := applyUnifiedDiff(original, diff)
	if err == nil {
		t.Fatal("expected error for context mismatch, got nil")
	}
}

func TestApplyUnifiedDiff_NoHunks(t *testing.T) {
	_, err := applyUnifiedDiff("hello\n", "just some random text\n")
	if err == nil {
		t.Fatal("expected error for no hunks, got nil")
	}
}

func TestApplyUnifiedDiff_GitDiffFormat(t *testing.T) {
	original := "func hello() {\n\tfmt.Println(\"hello\")\n}\n"
	diff := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 func hello() {
-	fmt.Println("hello")
+	fmt.Println("world")
 }
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "func hello() {\n\tfmt.Println(\"world\")\n}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_AddAtEnd(t *testing.T) {
	original := "line1\nline2\n"
	diff := `@@ -1,2 +1,3 @@
 line1
 line2
+line3
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline2\nline3\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_BareHunkHeader(t *testing.T) {
	original := "- line1\n- line2\n- line3\n"
	diff := `@@
 - line1
+- line-new
 - line2
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `- line1
- line-new
- line2
- line3
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_BareHunkHeaderMiddle(t *testing.T) {
	// Context lines match in the middle of the file, not at line 1.
	original := "aaa\nbbb\nccc\nddd\neee\n"
	diff := `@@
 ccc
-ddd
+DDD
 eee
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "aaa\nbbb\nccc\nDDD\neee\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_BareHunkHeaderPureAdd(t *testing.T) {
	// Bare @@ with only additions and no context — inserts at the beginning.
	original := "existing\n"
	diff := `@@
+new-line
`
	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "new-line\nexisting\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyUnifiedDiff_MultipleBareHunks(t *testing.T) {
	original := "a\nb\nc\nd\ne\nf\n"
	diff := `@@
 b
-c
+C
 d
@@
 e
+3
 f
`

	got, err := applyUnifiedDiff(original, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a\nb\nC\nd\ne\n3\nf\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line    string
		wantOld int
		wantCnt int
		wantErr bool
	}{
		{"@@ -1,5 +1,6 @@", 1, 5, false},
		{"@@ -10 +10,2 @@", 10, 1, false},
		{"@@ -1,3 +1,4 @@ func main()", 1, 3, false},
		{"@@", 0, 0, false}, // bare hunk header — position inferred from content
		{"not a hunk", 0, 0, true},
	}

	for _, tt := range tests {
		h, err := parseHunkHeader(tt.line)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseHunkHeader(%q): expected error", tt.line)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHunkHeader(%q): unexpected error: %v", tt.line, err)
			continue
		}
		if h.oldStart != tt.wantOld || h.oldCount != tt.wantCnt {
			t.Errorf("parseHunkHeader(%q): got (%d,%d), want (%d,%d)",
				tt.line, h.oldStart, h.oldCount, tt.wantOld, tt.wantCnt)
		}
	}
}

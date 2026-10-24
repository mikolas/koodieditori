package tooling

import "testing"

func TestJumplistEmpty(t *testing.T) {
	jl := NewJumplist(10)
	if jl.Len() != 0 {
		t.Fatalf("expected empty jumplist, got len %d", jl.Len())
	}
	if _, ok := jl.Back(); ok {
		t.Fatal("expected Back on empty jumplist to return false")
	}
	if _, ok := jl.Forward(); ok {
		t.Fatal("expected Forward on empty jumplist to return false")
	}
	if _, ok := jl.Current(); ok {
		t.Fatal("expected Current on empty jumplist to return false")
	}
}

func TestJumplistRecordAndTraverse(t *testing.T) {
	jl := NewJumplist(5)

	loc1 := Location{FilePath: "main.go", Line: 10, Col: 0}
	loc2 := Location{FilePath: "main.go", Line: 50, Col: 5}
	loc3 := Location{FilePath: "util.go", Line: 100, Col: 2}

	jl.Record(loc1)
	jl.Record(loc1) // Duplicate should be ignored
	if jl.Len() != 1 {
		t.Fatalf("expected len 1, got %d", jl.Len())
	}

	jl.Record(loc2)
	jl.Record(loc3)
	if jl.Len() != 3 {
		t.Fatalf("expected len 3, got %d", jl.Len())
	}

	// Currently at loc3
	curr, ok := jl.Current()
	if !ok || curr != loc3 {
		t.Fatalf("expected curr to be loc3, got %+v", curr)
	}

	// Back to loc2
	back1, ok := jl.Back()
	if !ok || back1 != loc2 {
		t.Fatalf("expected Back to return loc2, got %+v", back1)
	}

	// Back to loc1
	back2, ok := jl.Back()
	if !ok || back2 != loc1 {
		t.Fatalf("expected Back to return loc1, got %+v", back2)
	}

	// Cannot go back further
	if _, ok := jl.Back(); ok {
		t.Fatal("expected Back at start to return false")
	}

	// Forward to loc2
	fwd1, ok := jl.Forward()
	if !ok || fwd1 != loc2 {
		t.Fatalf("expected Forward to return loc2, got %+v", fwd1)
	}

	// Record new location from loc2 -> should truncate loc3
	loc4 := Location{FilePath: "extra.go", Line: 1, Col: 1}
	jl.Record(loc4)
	if jl.Len() != 3 { // loc1, loc2, loc4
		t.Fatalf("expected len 3 after branch, got %d", jl.Len())
	}

	// Forward should now fail because we're at the end (loc4)
	if _, ok := jl.Forward(); ok {
		t.Fatal("expected Forward at end of branch to return false")
	}

	curr, _ = jl.Current()
	if curr != loc4 {
		t.Fatalf("expected current to be loc4, got %+v", curr)
	}
}

func TestJumplistMaxSizeCap(t *testing.T) {
	jl := NewJumplist(3)
	jl.Record(Location{FilePath: "1", Line: 1, Col: 1})
	jl.Record(Location{FilePath: "2", Line: 2, Col: 2})
	jl.Record(Location{FilePath: "3", Line: 3, Col: 3})
	jl.Record(Location{FilePath: "4", Line: 4, Col: 4})

	if jl.Len() != 3 {
		t.Fatalf("expected len capped at 3, got %d", jl.Len())
	}

	// Back should reach 3, then 2, but 1 was evicted
	b1, _ := jl.Back()
	if b1.FilePath != "3" {
		t.Errorf("expected 3, got %s", b1.FilePath)
	}
	b2, _ := jl.Back()
	if b2.FilePath != "2" {
		t.Errorf("expected 2, got %s", b2.FilePath)
	}
	if _, ok := jl.Back(); ok {
		t.Error("expected 1 to have been evicted")
	}
}

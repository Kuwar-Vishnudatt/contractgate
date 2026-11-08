package semver

import "testing"

func TestParseValid(t *testing.T) {
	v, err := Parse("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 2, 3}) {
		t.Fatalf("got %+v", v)
	}
}

func TestParseWithVPrefix(t *testing.T) {
	v, err := Parse("v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{2, 0, 0}) {
		t.Fatalf("got %+v", v)
	}
}

func TestParseRejectsWrongShape(t *testing.T) {
	for _, s := range []string{"1.2", "1.2.3.4", "abc", "", "1.2.x"} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("expected error parsing %q", s)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.2.0", "1.3.0", -1},
		{"1.2.5", "1.2.4", 1},
		{"2.0.0", "1.9.9", 1}, // major dominates minor/patch
	}
	for _, c := range cases {
		a, _ := Parse(c.a)
		b, _ := Parse(c.b)
		if got := Compare(a, b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsMajorBump(t *testing.T) {
	old, _ := Parse("1.5.2")
	cases := []struct {
		newV string
		want bool
	}{
		{"2.0.0", true},
		{"2.0.0", true},
		{"1.6.0", false},
		{"1.5.3", false},
		{"1.0.0", false}, // a major DOWNGRADE is not a major bump
	}
	for _, c := range cases {
		nv, _ := Parse(c.newV)
		if got := IsMajorBump(old, nv); got != c.want {
			t.Errorf("IsMajorBump(1.5.2, %s) = %v, want %v", c.newV, got, c.want)
		}
	}
}

func TestIsAnyBump(t *testing.T) {
	old, _ := Parse("1.5.2")
	same, _ := Parse("1.5.2")
	higher, _ := Parse("1.5.3")
	lower, _ := Parse("1.5.1")
	if IsAnyBump(old, same) {
		t.Error("identical versions must not count as a bump")
	}
	if !IsAnyBump(old, higher) {
		t.Error("a patch increase must count as a bump")
	}
	if IsAnyBump(old, lower) {
		t.Error("a version decrease must not count as a bump")
	}
}

package version

import "testing"

func TestGetReturnsInjectedValues(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origV, origC, origD })

	Version, Commit, Date = "v1.2.3", "abc1234", "2026-10-01T00:00:00Z"

	got := Get()
	want := Info{Version: "v1.2.3", Commit: "abc1234", Date: "2026-10-01T00:00:00Z"}
	if got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}
	if s := got.String(); s != "v1.2.3 (abc1234, 2026-10-01T00:00:00Z)" {
		t.Fatalf("String() = %q", s)
	}
}

func TestDefaultsAreDev(t *testing.T) {
	if Version != "dev" || Commit != "none" || Date != "unknown" {
		t.Fatalf("unexpected defaults: %s %s %s", Version, Commit, Date)
	}
}

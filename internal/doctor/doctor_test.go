package doctor

import (
	"context"
	"testing"
)

type mockCheck struct {
	id     string
	status Status
	fixed  bool
}

func (m *mockCheck) Run(_ context.Context) Result {
	s := m.status
	if m.fixed {
		s = StatusOK
	}
	return Result{ID: m.id, Status: s, FixAvailable: m.status != StatusOK}
}

func (m *mockCheck) Fix(_ context.Context) error {
	m.fixed = true
	return nil
}

func TestRunner_Run(t *testing.T) {
	r := &Runner{Checks: []Check{
		&mockCheck{id: "a", status: StatusOK},
		&mockCheck{id: "b", status: StatusWarning},
	}}
	results := r.Run(context.Background())
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Errorf("IDs preserved order: %+v", results)
	}
}

func TestRunner_RunAndFix(t *testing.T) {
	r := &Runner{Checks: []Check{
		&mockCheck{id: "needs-fix", status: StatusError},
		&mockCheck{id: "ok", status: StatusOK},
	}}
	results := r.RunAndFix(context.Background())
	if results[0].Status != StatusOK {
		t.Errorf("RunAndFix should have flipped first check to OK, got %+v", results[0])
	}
}

func TestExit_CodeMatrix(t *testing.T) {
	cases := []struct {
		name string
		res  []Result
		want int
	}{
		{"all ok", []Result{{Status: StatusOK}, {Status: StatusOK}}, 0},
		{"warning only", []Result{{Status: StatusWarning}}, 0},
		{"any error", []Result{{Status: StatusOK}, {Status: StatusError}}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Exit(c.res); got != c.want {
				t.Errorf("Exit = %d, want %d", got, c.want)
			}
		})
	}
}

func TestJSON_ContainsSummary(t *testing.T) {
	res := []Result{
		{ID: "a", Status: StatusOK},
		{ID: "b", Status: StatusError},
	}
	b, err := JSON(res)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"summary", "results", `"error": 1`, `"ok": 1`} {
		if !containsString(s, want) {
			t.Errorf("JSON missing %q in:\n%s", want, s)
		}
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDefaultRunner_ReturnsChecks(t *testing.T) {
	r := DefaultRunner()
	if len(r.Checks) == 0 {
		t.Error("DefaultRunner should populate platform-specific checks")
	}
	// Smoke-run them (should not panic even in hostile environments).
	_ = r.Run(context.Background())
}

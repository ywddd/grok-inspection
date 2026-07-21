package main

import "testing"

func TestAutoPolicyAction(t *testing.T) {
	cases := []struct {
		name string
		item accountResult
		want string
	}{
		{
			name: "permission denied always delete",
			item: accountResult{Classification: "permission_denied", Disabled: false},
			want: "delete",
		},
		{
			name: "permission denied already disabled still delete",
			item: accountResult{Classification: "permission_denied", Disabled: true},
			want: "delete",
		},
		{
			name: "quota exhausted enabled → disable",
			item: accountResult{Classification: "quota_exhausted", Disabled: false},
			want: "disable",
		},
		{
			name: "quota exhausted already disabled → skip",
			item: accountResult{Classification: "quota_exhausted", Disabled: true},
			want: "",
		},
		{
			name: "healthy enabled → skip",
			item: accountResult{Classification: "healthy", Disabled: false},
			want: "",
		},
		{
			name: "healthy disabled → enable",
			item: accountResult{Classification: "healthy", Disabled: true},
			want: "enable",
		},
		{
			name: "reauth never auto",
			item: accountResult{Classification: "reauth", Action: "delete", Disabled: false},
			want: "",
		},
		{
			name: "probe error never auto",
			item: accountResult{Classification: "probe_error", Disabled: false},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoPolicyAction(tc.item); got != tc.want {
				t.Fatalf("autoPolicyAction(%+v) = %q, want %q", tc.item, got, tc.want)
			}
		})
	}
}

func TestCollectAutoPolicyCandidates(t *testing.T) {
	results := []accountResult{
		{Name: "a", Classification: "permission_denied", Disabled: false},
		{Name: "b", Classification: "quota_exhausted", Disabled: false},
		{Name: "c", Classification: "quota_exhausted", Disabled: true},
		{Name: "d", Classification: "healthy", Disabled: true},
		{Name: "e", Classification: "healthy", Disabled: false},
		{Name: "f", Classification: "reauth", Disabled: false},
	}
	got := collectAutoPolicyCandidates(results)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %+v", len(got), got)
	}
	want := map[string]string{"a": "delete", "b": "disable", "d": "enable"}
	for _, item := range got {
		if want[item.Name] != item.Action {
			t.Fatalf("name=%s action=%s want=%s", item.Name, item.Action, want[item.Name])
		}
	}
}

func TestNormalizeScheduleInterval(t *testing.T) {
	got, err := normalizeScheduleInterval(0)
	if err != nil || got != defaultScheduleIntervalMin {
		t.Fatalf("zero default: got=%d err=%v", got, err)
	}
	if _, err := normalizeScheduleInterval(-1); err == nil {
		t.Fatal("expected error for negative")
	}
	if _, err := normalizeScheduleInterval(maxScheduleIntervalMin + 1); err == nil {
		t.Fatal("expected error for too large")
	}
	got, err = normalizeScheduleInterval(10)
	if err != nil || got != 10 {
		t.Fatalf("10: got=%d err=%v", got, err)
	}
}

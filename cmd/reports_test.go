package cmd

import (
	"net/url"
	"testing"
	"time"
)

func TestResolveReportPeriod(t *testing.T) {
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		flags    reportFlags
		path     string
		from, to string
		err      bool
	}{
		{name: "default is the last 30 days", flags: reportFlags{}, path: "/v1/api/reports/range", from: "2026-09-15", to: "2026-10-14"},
		{name: "last 7 days", flags: reportFlags{last: "7d"}, path: "/v1/api/reports/range", from: "2026-10-08", to: "2026-10-14"},
		{name: "month", flags: reportFlags{month: "2026-09"}, path: "/v1/api/reports/monthly/2026-09-01"},
		{name: "quarter", flags: reportFlags{quarter: "2026-Q3"}, path: "/v1/api/reports/quarterly/2026-07-01"},
		{name: "range", flags: reportFlags{from: "2026-09-01", to: "2026-09-15"}, path: "/v1/api/reports/range", from: "2026-09-01", to: "2026-09-15"},
		{name: "this week", flags: reportFlags{thisWeek: true}, path: "/v1/api/reports/live"},
		{name: "sent week is looked up", flags: reportFlags{week: true}, path: ""},
		{name: "two periods", flags: reportFlags{month: "2026-09", last: "7d"}, err: true},
		{name: "bad month", flags: reportFlags{month: "2026-13"}, err: true},
		{name: "bad quarter", flags: reportFlags{quarter: "2026-Q5"}, err: true},
		{name: "half a range", flags: reportFlags{from: "2026-09-01"}, err: true},
		{name: "bad last", flags: reportFlags{last: "4w"}, err: true},
		{name: "bad csv", flags: reportFlags{csv: "weeks"}, err: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := url.Values{}
			path, err := resolveReportPeriod(c.flags, today, q)
			if c.err {
				if err == nil {
					t.Fatalf("want an error, got %q", path)
				}
				return
			}
			if err != nil || path != c.path || q.Get("from") != c.from || q.Get("to") != c.to {
				t.Fatalf("got %q %v (err %v), want %q from=%s to=%s", path, q, err, c.path, c.from, c.to)
			}
		})
	}
}

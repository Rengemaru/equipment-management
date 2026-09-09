package jst

import (
	"testing"
	"time"
)

func TestDate_UTCの前日に落ちない(t *testing.T) {
	// JST 00:30 は UTC では前日の 15:30。UTCのまま日付に落とすと1日ずれる。
	at := time.Date(2026, 9, 10, 0, 30, 0, 0, Zone)

	if got := FormatDate(at); got != "2026-09-10" {
		t.Errorf("FormatDate = %q, want 2026-09-10", got)
	}
	if got := FormatDate(at.UTC()); got != "2026-09-10" {
		t.Errorf("UTCで渡した場合 = %q, want 2026-09-10", got)
	}
}

func TestParseDate_JSTの0時として読む(t *testing.T) {
	got, err := ParseDate("2026-09-10")
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}

	want := time.Date(2026, 9, 10, 0, 0, 0, 0, Zone)
	if !got.Equal(want) {
		t.Errorf("ParseDate = %v, want %v", got, want)
	}
}

func TestParseDate_読めない形式は失敗する(t *testing.T) {
	if _, err := ParseDate("2026/09/10"); err == nil {
		t.Error("2026/09/10 が読めてしまった")
	}
}

func TestDaysBetween(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		to   time.Time
		want int
	}{
		{
			"同じ日",
			time.Date(2026, 9, 10, 0, 0, 0, 0, Zone),
			time.Date(2026, 9, 10, 23, 59, 0, 0, Zone),
			0,
		},
		{
			// 23時と翌0時の差は1時間だが、日付は1日違う。
			// 24時間で割る実装だとここが 0 になる。
			"23時から翌0時",
			time.Date(2026, 9, 10, 23, 0, 0, 0, Zone),
			time.Date(2026, 9, 11, 0, 0, 0, 0, Zone),
			1,
		},
		{
			"過去は負",
			time.Date(2026, 9, 10, 0, 0, 0, 0, Zone),
			time.Date(2026, 9, 8, 0, 0, 0, 0, Zone),
			-2,
		},
		{
			"月をまたぐ",
			time.Date(2026, 8, 31, 12, 0, 0, 0, Zone),
			time.Date(2026, 9, 1, 1, 0, 0, 0, Zone),
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DaysBetween(tt.from, tt.to); got != tt.want {
				t.Errorf("DaysBetween = %d, want %d", got, tt.want)
			}
		})
	}
}

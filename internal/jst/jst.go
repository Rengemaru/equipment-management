// Package jst は日本時間の扱いを1箇所に閉じ込める。
//
// 保存はUTC、日付の解釈はJST。返却予定日や期限超過は「日付」で決まるため、
// UTCのまま計算すると JST 00:00〜09:00 の分が1日ずれる。
//
// **time.LoadLocation("Asia/Tokyo") を使わないこと。** 本番イメージは scratch で
// tzdata が入っておらず、実行時に失敗する。time/tzdata を埋め込む手もあるが、
// 日本以外で使う予定がないため固定オフセットの方が安い。
package jst

import "time"

// Zone は日本標準時。夏時間が無いため固定オフセットで足りる。
var Zone = time.FixedZone("JST", 9*60*60)

// DateLayout は日付だけを持つ列（loans.due_date など）の形式。
const DateLayout = "2006-01-02"

// Date は時刻をJSTの日付に落とす。時刻部分は 00:00:00 になる。
func Date(t time.Time) time.Time {
	t = t.In(Zone)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Zone)
}

// FormatDate は時刻をJSTの日付文字列にする。
func FormatDate(t time.Time) string {
	return t.In(Zone).Format(DateLayout)
}

// ParseDate は 'YYYY-MM-DD' をJSTのその日の 00:00 として読む。
//
// time.Parse で読むとUTCの0時になり、JSTに直すと前日の9時になる。
// 日付同士の比較にそのまま使える値を返す。
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, s, Zone)
}

// DaysBetween は日付の差を日数で返す。from より to が後なら正。
//
// 時刻を含む値を渡されても日付に落としてから引く。24時間で割ると、
// 夏時間の無いJSTでも「23時に借りて翌0時」のような境目で1日ずれる。
func DaysBetween(from, to time.Time) int {
	d := Date(to).Sub(Date(from))
	return int(d.Hours() / 24)
}

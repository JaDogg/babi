package dt

import (
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── Duration ────────────────────────────────────────────────────────────────

// CalDuration is a calendar-aware duration (years/months are inexact by nature).
type CalDuration struct {
	Neg     bool
	Years   int
	Months  int
	Weeks   int
	Days    int
	Hours   int
	Minutes int
	Seconds int
}

var durRe = regexp.MustCompile(`(\d+)(y|mo|w|d|h|m|s)`)

// ParseCalDuration parses strings like "1h", "2d", "-1mo", "1y2mo3d4h5m6s".
func ParseCalDuration(s string) (CalDuration, error) {
	var d CalDuration
	if strings.HasPrefix(s, "-") {
		d.Neg = true
		s = s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	matches := durRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return d, fmt.Errorf("invalid duration %q (examples: 1h, 2d, 1mo, 1y2mo3d4h)", s)
	}
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "y":
			d.Years = n
		case "mo":
			d.Months = n
		case "w":
			d.Weeks = n
		case "d":
			d.Days = n
		case "h":
			d.Hours = n
		case "m":
			d.Minutes = n
		case "s":
			d.Seconds = n
		}
	}
	return d, nil
}

// AddTo applies the duration to t (positive or negative).
func (d CalDuration) AddTo(t time.Time) time.Time {
	sign := 1
	if d.Neg {
		sign = -1
	}
	t = t.AddDate(sign*d.Years, sign*d.Months, sign*(d.Weeks*7+d.Days))
	return t.Add(time.Duration(sign) * (
		time.Duration(d.Hours)*time.Hour +
			time.Duration(d.Minutes)*time.Minute +
			time.Duration(d.Seconds)*time.Second))
}

// String returns the canonical string form, e.g. "-1y2mo3d".
func (d CalDuration) String() string {
	var b strings.Builder
	if d.Neg {
		b.WriteByte('-')
	}
	for _, p := range []struct {
		n    int
		unit string
	}{
		{d.Years, "y"}, {d.Months, "mo"}, {d.Weeks, "w"},
		{d.Days, "d"}, {d.Hours, "h"}, {d.Minutes, "m"}, {d.Seconds, "s"},
	} {
		if p.n != 0 {
			fmt.Fprintf(&b, "%d%s", p.n, p.unit)
		}
	}
	return b.String()
}

// ─── Age ─────────────────────────────────────────────────────────────────────

// Age holds a human-readable age breakdown.
type Age struct {
	Years      int
	Months     int
	Days       int
	TotalDays  int
	TotalHours int
	Birth      time.Time
}

// CalcAge computes the age since birth as of now.
func CalcAge(birth time.Time) Age {
	now := time.Now()
	years := now.Year() - birth.Year()
	months := int(now.Month()) - int(birth.Month())
	days := now.Day() - birth.Day()

	if days < 0 {
		months--
		prev := now.AddDate(0, -1, 0)
		days += daysInMonth(prev.Year(), prev.Month())
	}
	if months < 0 {
		years--
		months += 12
	}
	totalDays := int(now.Sub(birth).Hours() / 24)
	return Age{
		Years:      years,
		Months:     months,
		Days:       days,
		TotalDays:  totalDays,
		TotalHours: int(now.Sub(birth).Hours()),
		Birth:      birth,
	}
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ParseDate parses common date formats.
func ParseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"02-01-2006",
		"02/01/2006",
		"January 2, 2006",
		"Jan 2, 2006",
		"Jan 2 2006",
		"2 January 2006",
		"2 Jan 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q (use YYYY-MM-DD)", s)
}

// ─── NTP ─────────────────────────────────────────────────────────────────────

const ntpEpochOffset = 2208988800 // seconds between 1900-01-01 and 1970-01-01

// DefaultNTPServers is the list queried by QueryAll.
var DefaultNTPServers = []string{
	"pool.ntp.org",
	"time.google.com",
	"time.cloudflare.com",
	"time.apple.com",
}

// NTPResult holds the result of a single NTP query.
type NTPResult struct {
	Server string
	Time   time.Time     // server time
	Offset time.Duration // local clock offset (positive = local is behind)
	RTT    time.Duration // round-trip time
	Err    error
}

// QueryNTP queries a single NTP server.
func QueryNTP(server string) NTPResult {
	res := NTPResult{Server: server}

	addr, err := net.ResolveUDPAddr("udp", server+":123")
	if err != nil {
		res.Err = err
		return res
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		res.Err = err
		return res
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	req := make([]byte, 48)
	req[0] = 0x1B // LI=0, VN=3, Mode=3 (client)

	t1 := time.Now()
	if _, err := conn.Write(req); err != nil {
		res.Err = err
		return res
	}

	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		res.Err = err
		return res
	}
	t4 := time.Now()

	// Transmit timestamp (T3) is at bytes 40–47
	secs := binary.BigEndian.Uint32(resp[40:44])
	frac := binary.BigEndian.Uint32(resp[44:48])
	t3 := time.Unix(int64(secs)-ntpEpochOffset, int64(frac)*1e9/(1<<32)).UTC()

	// Clock offset = T3 - midpoint(T1,T4)
	rtt := t4.Sub(t1)
	mid := t1.Add(rtt / 2)
	res.Time = t3
	res.Offset = t3.Sub(mid)
	res.RTT = rtt
	return res
}

// QueryAll queries all DefaultNTPServers in parallel.
func QueryAll() []NTPResult {
	results := make([]NTPResult, len(DefaultNTPServers))
	var wg sync.WaitGroup
	for i, s := range DefaultNTPServers {
		wg.Add(1)
		go func(i int, s string) {
			defer wg.Done()
			results[i] = QueryNTP(s)
		}(i, s)
	}
	wg.Wait()
	return results
}

// AverageOffset returns the mean offset from successful NTP results.
func AverageOffset(results []NTPResult) (time.Duration, bool) {
	var sum time.Duration
	n := 0
	for _, r := range results {
		if r.Err == nil {
			sum += r.Offset
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / time.Duration(n), true
}

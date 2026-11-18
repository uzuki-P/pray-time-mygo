package adhan

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testdata/singapore-golden.txt holds the times Adhan Java 1.2.1 computes
// with CalculationMethod.SINGAPORE and Madhab.SHAFI (truncated to minutes, as
// the Kotlin app stored them): latitude longitude date fajr sunrise dhuhr asr
// maghrib isha. The port must match every one.
func TestMatchesAdhanJava(t *testing.T) {
	f, err := os.Open("testdata/singapore-golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	cases := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 9 {
			continue
		}
		lat, _ := strconv.ParseFloat(fields[0], 64)
		lon, _ := strconv.ParseFloat(fields[1], 64)
		date, err := time.Parse("2006-01-02", fields[2])
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Calculate(lat, lon, date.Year(), date.Month(), date.Day(), Singapore())
		if !ok {
			t.Errorf("%s: no times", scanner.Text())
			continue
		}
		names := []string{"fajr", "sunrise", "dhuhr", "asr", "maghrib", "isha"}
		values := []time.Time{got.Fajr, got.Sunrise, got.Dhuhr, got.Asr, got.Maghrib, got.Isha}
		for i, want := range fields[3:] {
			if g := values[i].UTC().Format("2006-01-02T15:04:05Z"); g != want {
				t.Errorf("%s %s %s %s: got %s, want %s", fields[0], fields[1], fields[2], names[i], g, want)
			}
		}
		cases++
	}
	if cases < 60 {
		t.Fatalf("only %d golden cases", cases)
	}
}

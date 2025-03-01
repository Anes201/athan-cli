package main

import (
        "encoding/json"
        "flag"
        "fmt"
        "io"
        "net/http"
        "os"
        "sort"
        "time"
)

type PrayerTimes struct {
        Code int `json:"code"`
        Data struct {
                Timings map[string]string `json:"timings"`
                Date    struct {
                        Readable string `json:"readable"`
                        Hijri    struct {
                                Readable string `json:"readable"`
                        } `json:"hijri"`
                } `json:"date"`
        } `json:"data"`
}

type GeocodeResponse struct {
        Results []struct {
                Geometry struct {
                        Location struct {
                                Lat float64 `json:"lat"`
                                Lng float64 `json:"lng"`
                        } `json:"location"`
                } `json:"geometry"`
        } `json:"results"`
}

type Prayer struct {
        Name string
        Time string
}

func getPrayerTimes(latitude, longitude float64, method int) (*PrayerTimes, error) {
        today := time.Now().Format("02-01-2006")
        url := fmt.Sprintf("http://api.aladhan.com/v1/timings/%s?latitude=%f&longitude=%f&method=%d", today, latitude, longitude, method)

        resp, err := http.Get(url)
        if err != nil {
                return nil, fmt.Errorf("HTTP request failed: %w", err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return nil, fmt.Errorf("failed to read response body: %w", err)
        }

        if resp.StatusCode != http.StatusOK {
                return nil, fmt.Errorf("HTTP request returned status: %s, body: %s", resp.Status, string(body))
        }

        var prayerTimes PrayerTimes
        err = json.Unmarshal(body, &prayerTimes)
        if err != nil {
                return nil, fmt.Errorf("failed to decode JSON: %w", err)
        }

        if prayerTimes.Code != 200 {
                return nil, fmt.Errorf("API returned code: %d", prayerTimes.Code)
        }

        return &prayerTimes, nil
}

func parseTime(timeStr string) (time.Time, error) {
        return time.Parse("15:04", timeStr)
}

func calculateTimeUntilNextPrayer(prayerTimes map[string]string) (string, time.Duration, error) {
        now := time.Now()

        var nextPrayerName string
        minDuration := time.Duration(1<<63 - 1) // Max Duration

        for prayerName, prayerTimeStr := range prayerTimes {
                prayerTime, err := parseTime(prayerTimeStr)
                if err != nil {
                        return "", 0, err
                }

                prayerTime = time.Date(now.Year(), now.Month(), now.Day(), prayerTime.Hour(), prayerTime.Minute(), 0, 0, now.Location())

                duration := prayerTime.Sub(now)
                if duration < 0 {
                        prayerTime = prayerTime.Add(24 * time.Hour)
                        duration = prayerTime.Sub(now)
                }

                if duration < minDuration {
                        minDuration = duration
                        nextPrayerName = prayerName
                }
        }

        return nextPrayerName, minDuration, nil
}

func geocodeCity(city string) (float64, float64, error) {
        apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
        if apiKey == "" {
                return 0, 0, fmt.Errorf("GOOGLE_MAPS_API_KEY environment variable not set")
        }

        url := fmt.Sprintf("https://maps.googleapis.com/maps/api/geocode/json?address=%s&key=%s", city, apiKey)
        resp, err := http.Get(url)
        if err != nil {
                return 0, 0, fmt.Errorf("geocode HTTP request failed: %w", err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return 0, 0, fmt.Errorf("failed to read geocode response body: %w", err)
        }

        var geocodeResponse GeocodeResponse
        err = json.Unmarshal(body, &geocodeResponse)
        if err != nil {
                return 0, 0, fmt.Errorf("failed to decode geocode JSON: %w", err)
        }

        if len(geocodeResponse.Results) == 0 {
                return 0, 0, fmt.Errorf("city not found")
        }

        lat := geocodeResponse.Results[0].Geometry.Location.Lat
        lng := geocodeResponse.Results[0].Geometry.Location.Lng

        return lat, lng, nil
}

func main() {
        city := flag.String("city", "", "City name for prayer times")
        lat := flag.Float64("lat", 0, "Latitude for prayer times")
        lng := flag.Float64("lng", 0, "Longitude for prayer times")
        method := flag.Int("method", 19, "Calculation method")

        flag.Parse()

        var latitude, longitude float64
        var err error

        if *city != "" {
                latitude, longitude, err = geocodeCity(*city)
                if err != nil {
                        fmt.Println("Error:", err)
                        return
                }
        } else if *lat != 0 && *lng != 0 {
                latitude, longitude = *lat, *lng
        } else {
                fmt.Println("Please provide either -city or -lat and -lng")
                flag.Usage()
                return
        }

        prayerTimes, err := getPrayerTimes(latitude, longitude, *method)
        if err != nil {
                fmt.Println("Error:", err)
                return
        }

        fmt.Println("Islamic Prayer Times:")
        fmt.Printf("Date: %s\n", prayerTimes.Data.Date.Readable)
        // fmt.Printf("Hijri Date: %s\n\n", prayerTimes.Data.Date.Hijri.Readable)
        fmt.Println("----------------------")

        var prayers []Prayer
        for prayerName, prayerTime := range prayerTimes.Data.Timings {
                prayers = append(prayers, Prayer{Name: prayerName, Time: prayerTime})
        }

        sort.Slice(prayers, func(i, j int) bool {
                timeI, _ := parseTime(prayers[i].Time)
                timeJ, _ := parseTime(prayers[j].Time)

                now := time.Now()
                timeI = time.Date(now.Year(), now.Month(), now.Day(), timeI.Hour(), timeI.Minute(), 0, 0, now.Location())
                timeJ = time.Date(now.Year(), now.Month(), now.Day(), timeJ.Hour(), timeJ.Minute(), 0, 0, now.Location())

                return timeI.Before(timeJ)
        })

        for _, prayer := range prayers {
                fmt.Printf("%-8s \t: %s\n", prayer.Name, prayer.Time)
        }

        nextPrayerName, duration, err := calculateTimeUntilNextPrayer(prayerTimes.Data.Timings)
        if err != nil {
                fmt.Println("Error calculating time until next prayer:", err)
                return
        }

        hours := int(duration.Hours())
        minutes := int(duration.Minutes()) % 60
        seconds := int(duration.Seconds()) % 60

        fmt.Printf("\nTime Until Next Prayer (%s): %02d:%02d:%02d\n", nextPrayerName, hours, minutes, seconds)
}

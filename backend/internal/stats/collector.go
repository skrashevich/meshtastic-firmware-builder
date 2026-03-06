package stats

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

type EventType string

const (
	EventVisit    EventType = "visit"
	EventDiscover EventType = "discover"
	EventBuild    EventType = "build"
	EventDownload EventType = "download"
)

type Event struct {
	Timestamp time.Time `json:"ts"`
	Type      EventType `json:"type"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"ua,omitempty"`
	RepoURL   string    `json:"repo,omitempty"`
	Ref       string    `json:"ref,omitempty"`
	Device    string    `json:"device,omitempty"`
	Extra     string    `json:"extra,omitempty"`
}

type CountEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DayStats struct {
	Date      string `json:"date"`
	Visits    int    `json:"visits"`
	Discovers int    `json:"discovers"`
	Builds    int    `json:"builds"`
	Downloads int    `json:"downloads"`
}

type Summary struct {
	TotalVisits    int64        `json:"totalVisits"`
	TotalDiscovers int64        `json:"totalDiscovers"`
	TotalBuilds    int64        `json:"totalBuilds"`
	TotalDownloads int64        `json:"totalDownloads"`
	UniqueIPs      int64        `json:"uniqueIPs"`
	TopRepos       []CountEntry `json:"topRepos"`
	TopDevices     []CountEntry `json:"topDevices"`
	RecentEvents   []Event      `json:"recentEvents"`
	DailySummary   []DayStats   `json:"dailySummary"`
}

type Collector struct {
	filePath string
	mu       sync.Mutex
	logger   *log.Logger
	file     *os.File
}

func NewCollector(filePath string, logger *log.Logger) *Collector {
	return &Collector{
		filePath: filePath,
		logger:   logger,
	}
}

// getFile lazily opens the append-only write handle and reuses it.
// Must be called with c.mu held.
func (c *Collector) getFile() (*os.File, error) {
	if c.file != nil {
		return c.file, nil
	}
	f, err := os.OpenFile(c.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	c.file = f
	return f, nil
}

func (c *Collector) Record(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	line, err := json.Marshal(event)
	if err != nil {
		c.logger.Printf("stats: marshal event: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.getFile()
	if err != nil {
		c.logger.Printf("stats: open file: %v", err)
		return
	}

	if _, err := f.Write(append(line, '\n')); err != nil {
		c.logger.Printf("stats: write event: %v", err)
	}
}

type SummarizeOptions struct {
	RecentLimit int
	TopLimit    int
}

func (c *Collector) Summarize(opts SummarizeOptions) (Summary, error) {
	if opts.RecentLimit <= 0 {
		opts.RecentLimit = 50
	}
	if opts.TopLimit <= 0 {
		opts.TopLimit = 10
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.Open(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Summary{
				TopRepos:     []CountEntry{},
				TopDevices:   []CountEntry{},
				RecentEvents: []Event{},
				DailySummary: []DayStats{},
			}, nil
		}
		return Summary{}, err
	}
	defer f.Close()

	var (
		allEvents      []Event
		totalVisits    int64
		totalDiscovers int64
		totalBuilds    int64
		totalDownloads int64
		ipSet          = make(map[string]struct{})
		repoCounts     = make(map[string]int)
		deviceCounts   = make(map[string]int)
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB max line size
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		allEvents = append(allEvents, ev)
		ipSet[ev.IP] = struct{}{}

		switch ev.Type {
		case EventVisit:
			totalVisits++
		case EventDiscover:
			totalDiscovers++
			if ev.RepoURL != "" {
				repoCounts[ev.RepoURL]++
			}
		case EventBuild:
			totalBuilds++
			if ev.RepoURL != "" {
				repoCounts[ev.RepoURL]++
			}
			if ev.Device != "" {
				deviceCounts[ev.Device]++
			}
		case EventDownload:
			totalDownloads++
		}
	}
	if err := scanner.Err(); err != nil {
		return Summary{}, err
	}

	// Recent events (newest first)
	recent := make([]Event, 0, opts.RecentLimit)
	for i := len(allEvents) - 1; i >= 0 && len(recent) < opts.RecentLimit; i-- {
		recent = append(recent, allEvents[i])
	}

	// Top repos (by build+discover counts)
	topRepos := rankEntries(repoCounts, opts.TopLimit)

	// Top devices
	topDevices := rankEntries(deviceCounts, opts.TopLimit)

	// Daily summary (last 30 days)
	daily := buildDailySummary(allEvents, 30)

	return Summary{
		TotalVisits:    totalVisits,
		TotalDiscovers: totalDiscovers,
		TotalBuilds:    totalBuilds,
		TotalDownloads: totalDownloads,
		UniqueIPs:      int64(len(ipSet)),
		TopRepos:       topRepos,
		TopDevices:     topDevices,
		RecentEvents:   recent,
		DailySummary:   daily,
	}, nil
}

func rankEntries(counts map[string]int, limit int) []CountEntry {
	entries := make([]CountEntry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, CountEntry{Name: name, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func buildDailySummary(events []Event, days int) []DayStats {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Truncate(24 * time.Hour)
	dayMap := make(map[string]*DayStats)

	for _, ev := range events {
		if ev.Timestamp.Before(cutoff) {
			continue
		}
		date := ev.Timestamp.UTC().Format("2006-01-02")
		ds, ok := dayMap[date]
		if !ok {
			ds = &DayStats{Date: date}
			dayMap[date] = ds
		}
		switch ev.Type {
		case EventVisit:
			ds.Visits++
		case EventDiscover:
			ds.Discovers++
		case EventBuild:
			ds.Builds++
		case EventDownload:
			ds.Downloads++
		}
	}

	result := make([]DayStats, 0, len(dayMap))
	for _, ds := range dayMap {
		result = append(result, *ds)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})
	return result
}

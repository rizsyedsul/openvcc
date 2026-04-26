// Package chaos drives synthetic traffic at an Open VCC engine while
// programmatically failing one cloud, verifies the failover happened, and
// emits a JSON report. The output is the core artifact: a signed proof that
// "yesterday at 03:00 we lost AWS for 30 seconds and the SLO held".
package chaos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	AdminURL    string
	AdminToken  string
	ProxyURL    string
	FailCloud   string
	Duration    time.Duration
	RPS         int
	Concurrency int

	// Output is a convenience: when set and Sink is nil, the report is
	// JSON-encoded into Output. Use Sink for richer destinations.
	Output io.Writer
	Sink   Sink

	HTTPClient *http.Client
	Now        func() time.Time
}

type Report struct {
	StartedAt          time.Time      `json:"started_at"`
	EndedAt            time.Time      `json:"ended_at"`
	FailedCloud        string         `json:"failed_cloud"`
	Duration           time.Duration  `json:"duration"`
	Requests           int64          `json:"requests"`
	SuccessfulRequests int64          `json:"successful_requests"`
	FailedRequests     int64          `json:"failed_requests"`
	PerCloudCounts     map[string]int `json:"per_cloud_counts"`
	FailoverLag        time.Duration  `json:"failover_lag"`
	Result             string         `json:"result"`
	Reason             string         `json:"reason,omitempty"`
}

// Run executes a single chaos round. ctx cancellation aborts the run early
// (the cloud will still be re-enabled best-effort).
func Run(ctx context.Context, opts Options) (*Report, error) {
	if err := validate(&opts); err != nil {
		return nil, err
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	backends, err := fetchBackends(ctx, client, opts.AdminURL, opts.AdminToken)
	if err != nil {
		return nil, fmt.Errorf("list backends: %w", err)
	}
	failed := filterCloud(backends, opts.FailCloud)
	if len(failed) == 0 {
		return nil, fmt.Errorf("no backends matched cloud %q", opts.FailCloud)
	}

	report := &Report{
		StartedAt:      now(),
		FailedCloud:    opts.FailCloud,
		Duration:       opts.Duration,
		PerCloudCounts: make(map[string]int),
	}
	driver := newDriver(opts.ProxyURL, opts.RPS, opts.Concurrency, client)

	driveCtx, driveCancel := context.WithCancel(ctx)
	driverDone := driver.start(driveCtx)

	for _, name := range failed {
		if err := setHealth(ctx, client, opts.AdminURL, opts.AdminToken, name, false); err != nil {
			driveCancel()
			<-driverDone
			return nil, fmt.Errorf("mark %s unhealthy: %w", name, err)
		}
	}
	failureMarkedAt := now()

	timer := time.NewTimer(opts.Duration)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
	}

	for _, name := range failed {
		_ = setHealth(context.Background(), client, opts.AdminURL, opts.AdminToken, name, true)
	}
	driveCancel()
	<-driverDone

	report.EndedAt = now()
	report.Requests = driver.totalReqs.Load()
	report.SuccessfulRequests = driver.successReqs.Load()
	report.FailedRequests = driver.failedReqs.Load()
	driver.copyCloudCounts(report.PerCloudCounts)
	report.FailoverLag = driver.firstHitOtherCloud.Sub(failureMarkedAt)

	report.Result, report.Reason = evaluate(report, opts)
	sink := opts.Sink
	if sink == nil && opts.Output != nil {
		sink = NewWriterSink(opts.Output)
	}
	if sink != nil {
		if err := sink.Write(report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func evaluate(r *Report, opts Options) (string, string) {
	if r.Requests == 0 {
		return "fail", "no requests issued"
	}
	if hits := r.PerCloudCounts[r.FailedCloud]; hits > 0 {
		return "fail", fmt.Sprintf("traffic still hitting %s during failure window: %d responses",
			r.FailedCloud, hits)
	}
	if r.FailoverLag > opts.Duration/2 {
		return "fail", fmt.Sprintf("failover lag %v exceeded half the failure window", r.FailoverLag)
	}
	failedRatio := float64(r.FailedRequests) / float64(r.Requests)
	if failedRatio > 0.20 {
		return "fail", fmt.Sprintf("failed ratio %.2f%% > 20%%", failedRatio*100)
	}
	return "pass", ""
}

func validate(o *Options) error {
	switch {
	case o.AdminURL == "":
		return errors.New("AdminURL is required")
	case o.ProxyURL == "":
		return errors.New("ProxyURL is required")
	case o.FailCloud == "":
		return errors.New("FailCloud is required")
	case o.Duration <= 0:
		return errors.New("Duration must be > 0")
	}
	if o.RPS <= 0 {
		o.RPS = 50
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	return nil
}

type backendDTO struct {
	Name  string `json:"name"`
	Cloud string `json:"cloud"`
}

func fetchBackends(ctx context.Context, c *http.Client, adminURL, token string) ([]backendDTO, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", adminURL+"/admin/backends", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("admin returned %d: %s", resp.StatusCode, string(body))
	}
	var out []backendDTO
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func filterCloud(in []backendDTO, cloud string) []string {
	var out []string
	for _, b := range in {
		if b.Cloud == cloud {
			out = append(out, b.Name)
		}
	}
	return out
}

func setHealth(ctx context.Context, c *http.Client, adminURL, token, name string, healthy bool) error {
	body, _ := json.Marshal(map[string]bool{"healthy": healthy})
	req, err := http.NewRequestWithContext(ctx, "PUT",
		adminURL+"/admin/backends/"+name+"/health", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// driver issues GETs at ProxyURL at approximately RPS.
type driver struct {
	url         string
	rps         int
	concurrency int
	client      *http.Client

	totalReqs   atomic.Int64
	successReqs atomic.Int64
	failedReqs  atomic.Int64

	mu                 sync.Mutex
	cloudCounts        map[string]int
	firstHitOtherCloud time.Time
}

func newDriver(url string, rps, concurrency int, client *http.Client) *driver {
	return &driver{
		url:         url,
		rps:         rps,
		concurrency: concurrency,
		client:      client,
		cloudCounts: make(map[string]int),
	}
}

func (d *driver) start(ctx context.Context) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		interval := time.Second / time.Duration(d.rps)
		work := make(chan struct{}, d.concurrency*2)
		for i := 0; i < d.concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range work {
					d.fire(ctx)
				}
			}()
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				close(work)
				wg.Wait()
				return
			case <-t.C:
				select {
				case work <- struct{}{}:
				default:
				}
			}
		}
	}()
	return done
}

func (d *driver) fire(ctx context.Context) {
	d.totalReqs.Add(1)
	req, err := http.NewRequestWithContext(ctx, "GET", d.url, nil)
	if err != nil {
		d.failedReqs.Add(1)
		return
	}
	resp, err := d.client.Do(req)
	if err != nil {
		d.failedReqs.Add(1)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		d.failedReqs.Add(1)
		return
	}
	d.successReqs.Add(1)
	cloud := resp.Header.Get("X-Cloud")
	if cloud != "" {
		d.mu.Lock()
		d.cloudCounts[cloud]++
		if d.firstHitOtherCloud.IsZero() {
			d.firstHitOtherCloud = time.Now()
		}
		d.mu.Unlock()
	}
}

func (d *driver) copyCloudCounts(out map[string]int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, v := range d.cloudCounts {
		out[k] = v
	}
}

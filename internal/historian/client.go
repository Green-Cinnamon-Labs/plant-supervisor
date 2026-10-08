/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package historian is an HTTP client for tep-historian's POST /aggregate and
// POST /loop-performance endpoints.
package historian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SignalStats mirrors one entry of the historian's "signals" map. Statistics are nil when the
// window has no samples.
type SignalStats struct {
	Count int      `json:"count"`
	Mean  *float64 `json:"mean"`
	Std   *float64 `json:"std"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Last  *float64 `json:"last"`
}

// AggregateResponse mirrors the historian's /aggregate response.
type AggregateResponse struct {
	WindowS   float64                `json:"window_s"`
	Connected bool                   `json:"connected"`
	Signals   map[string]SignalStats `json:"signals"`
	Missing   []string               `json:"missing"`
}

// Means returns the window mean of every signal that has samples.
func (r *AggregateResponse) Means() map[string]float64 {
	means := make(map[string]float64, len(r.Signals))
	for k, s := range r.Signals {
		if s.Count > 0 && s.Mean != nil {
			means[k] = *s.Mean
		}
	}
	return means
}

// Client talks to one historian.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client with a short timeout: a slow historian should surface as a Pending plant,
// not block the reconciler.
func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 5 * time.Second}}
}

type aggregateRequest struct {
	Keys    []string `json:"keys"`
	WindowS float64  `json:"window_s"`
}

// Aggregate asks for window statistics of keys.
func (c *Client) Aggregate(ctx context.Context, keys []string, window time.Duration) (*AggregateResponse, error) {
	var out AggregateResponse
	if err := c.post(ctx, "/aggregate", aggregateRequest{Keys: keys, WindowS: window.Seconds()}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LoopSpec is one loop sent to POST /loop-performance.
type LoopSpec struct {
	Name          string  `json:"name"`
	PV            string  `json:"pv"`
	SP            float64 `json:"sp"`
	OP            string  `json:"op"`
	TimeConstantS float64 `json:"time_constant_s"`
}

// LoopPerformance mirrors one entry of the historian's "loops" map. PI is nil (with a Reason)
// when the index could not be computed.
type LoopPerformance struct {
	PI       *float64 `json:"pi"`
	Offset   *float64 `json:"offset"`
	PIRaw    *float64 `json:"pi_raw"`
	SigmaOP  *float64 `json:"sigma_op"`
	Reason   *string  `json:"reason"`
	Samples  int      `json:"n"`
	Horizon  int      `json:"b"`
	ARModelM int      `json:"m"`
}

// LoopPerformanceResponse mirrors the historian's /loop-performance response.
type LoopPerformanceResponse struct {
	Connected bool                       `json:"connected"`
	Loops     map[string]LoopPerformance `json:"loops"`
}

type loopPerformanceRequest struct {
	Loops           []LoopSpec `json:"loops"`
	WindowS         float64    `json:"window_s"`
	SampleIntervalS float64    `json:"sample_interval_s"`
}

// LoopPerformance asks for the Predictability Index of each loop over the window, with the
// series resampled to sampleInterval.
func (c *Client) LoopPerformance(ctx context.Context, loops []LoopSpec, window, sampleInterval time.Duration) (*LoopPerformanceResponse, error) {
	var out LoopPerformanceResponse
	req := loopPerformanceRequest{Loops: loops, WindowS: window.Seconds(), SampleIntervalS: sampleInterval.Seconds()}
	if err := c.post(ctx, "/loop-performance", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("historian %s returned %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding historian %s response: %w", path, err)
	}
	return nil
}

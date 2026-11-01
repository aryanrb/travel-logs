package services

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// open.er-api.com is free, keyless, covers NPR, and refreshes about once a day.
// Rates are quoted per 1 USD, so A→B = amount * rate[B] / rate[A].
const erAPIURL = "https://open.er-api.com/v6/latest/USD"

var rates = &rateCache{client: &http.Client{Timeout: 10 * time.Second}}

type rateCache struct {
	client *http.Client
	mu     sync.Mutex
	day    string             // UTC date of the cached rates
	perUSD map[string]float64 // code → units per 1 USD
}

// convertMinorUnits converts amount (minor units of `from`) into minor units of
// `to` using USD-based rates. Pure, so it is unit-testable without the network.
// ponytail: assumes both currencies use the same minor-unit exponent (true for
// NPR/USD/most; the web app labels the result "≈").
func convertMinorUnits(amount int64, perUSD map[string]float64, from, to string) (int64, bool) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to {
		return amount, true
	}
	rf, okf := perUSD[from]
	rt, okt := perUSD[to]
	if !okf || !okt || rf == 0 {
		return 0, false
	}
	return int64(math.Round(float64(amount) * rt / rf)), true
}

// convert converts using the daily rate cache, refetching at most once per UTC
// day. On a fetch failure it serves stale rates if any; otherwise ok=false.
func (c *rateCache) convert(amount int64, from, to string) (int64, bool) {
	if strings.EqualFold(from, to) {
		return amount, true
	}
	m, ok := c.ensure()
	if !ok {
		return 0, false
	}
	return convertMinorUnits(amount, m, from, to)
}

func (c *rateCache) ensure() (map[string]float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	if c.perUSD != nil && c.day == today {
		return c.perUSD, true
	}
	if m, ok := c.fetch(); ok {
		c.perUSD, c.day = m, today
		return m, true
	}
	if c.perUSD != nil { // stale is better than nothing
		return c.perUSD, true
	}
	return nil, false
}

func (c *rateCache) fetch() (map[string]float64, bool) {
	resp, err := c.client.Get(erAPIURL)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	return decodeRates(data)
}

// decodeRates pulls the rates map from an open.er-api response. Pure/testable.
func decodeRates(data []byte) (map[string]float64, bool) {
	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(data, &body); err != nil || body.Result != "success" || len(body.Rates) == 0 {
		return nil, false
	}
	return body.Rates, true
}

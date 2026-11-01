package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"travel-service/config"
	db "travel-service/internal/database"
	"travel-service/internal/models"
	"travel-service/internal/utils/apperror"
)

// validateCoords checks a map-pin lat/lng pair: both or neither, within WGS84
// ranges. A set pin always wins over a name lookup, so callers skip geocoding
// when these are present.
func validateCoords(lat, lng *float64) error {
	if (lat == nil) != (lng == nil) {
		return apperror.UnprocessableEntity("latitude and longitude must be provided together")
	}
	if lat != nil {
		if *lat < -90 || *lat > 90 {
			return apperror.UnprocessableEntity("latitude must be between -90 and 90")
		}
		if *lng < -180 || *lng > 180 {
			return apperror.UnprocessableEntity("longitude must be between -180 and 180")
		}
	}
	return nil
}

const nominatimURL = "https://nominatim.openstreetmap.org/search"

// geo is the process-wide geocoder. Nominatim's usage policy allows at most 1
// request/second and wants a descriptive User-Agent with a contact. Lookups are
// cached by place name; failures are ignored (coordinates stay null) and not
// cached, so a transient failure can be retried later.
var geo = &geocoder{client: &http.Client{Timeout: 10 * time.Second}, cache: map[string]coord{}}

type coord struct{ lat, lng float64 }

type geocoder struct {
	client *http.Client
	mu     sync.Mutex // serialises lookups to honour the 1 req/sec limit
	last   time.Time
	cache  map[string]coord
}

// geocodeDestination resolves a destination's coordinates in the background and
// saves them. Fire-and-forget: `go geocodeDestination(...)`.
func geocodeDestination(destID uuid.UUID, place string) {
	if c, ok := geo.lookup(place); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := models.New(db.Client).UpdateDestinationCoords(ctx, models.UpdateDestinationCoordsParams{
			Lat: &c.lat, Lng: &c.lng, ID: destID,
		}); err != nil {
			zap.L().Named("geocoder").Warn("save destination coords", zap.Error(err))
		}
	}
}

// geocodeTripStart resolves a trip's start_location coordinates in the background.
func geocodeTripStart(tripID uuid.UUID, place string) {
	if c, ok := geo.lookup(place); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := models.New(db.Client).UpdateTripStartCoords(ctx, models.UpdateTripStartCoordsParams{
			StartLat: &c.lat, StartLng: &c.lng, ID: tripID,
		}); err != nil {
			zap.L().Named("geocoder").Warn("save trip start coords", zap.Error(err))
		}
	}
}

// lookup returns cached coordinates or fetches them, holding the lock across the
// request so the whole process stays within 1 req/sec. Returns ok=false (and
// caches nothing) when geocoding is disabled or the lookup fails.
func (g *geocoder) lookup(place string) (coord, bool) {
	place = strings.TrimSpace(place)
	if place == "" || config.Env.NominatimContact == "" {
		return coord{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.cache[place]; ok {
		return c, true
	}
	if d := time.Since(g.last); d < time.Second {
		time.Sleep(time.Second - d)
	}
	g.last = time.Now()
	c, ok := g.fetch(place)
	if ok {
		g.cache[place] = c
	}
	return c, ok
}

func (g *geocoder) fetch(place string) (coord, bool) {
	u := nominatimURL + "?" + url.Values{"format": {"json"}, "limit": {"1"}, "q": {place}}.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return coord{}, false
	}
	req.Header.Set("User-Agent", "travelog/1.0 ("+config.Env.NominatimContact+")")
	resp, err := g.client.Do(req)
	if err != nil {
		return coord{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return coord{}, false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return coord{}, false
	}
	return decodeNominatim(data)
}

// decodeNominatim pulls the first result's lat/lng from a Nominatim response.
// Pure, so it is unit-testable without the network.
func decodeNominatim(data []byte) (coord, bool) {
	var body []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.Unmarshal(data, &body); err != nil || len(body) == 0 {
		return coord{}, false
	}
	lat, err1 := strconv.ParseFloat(body[0].Lat, 64)
	lng, err2 := strconv.ParseFloat(body[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return coord{}, false
	}
	return coord{lat, lng}, true
}

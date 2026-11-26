package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"
)

// realisticPage imita GET /api/v1/items (20 objetos con foto, zona y dueño).
func realisticPage() []byte {
	type photo struct {
		PublicID string `json:"publicId"`
		URL      string `json:"url"`
		ThumbURL string `json:"thumbUrl"`
	}
	type item struct {
		ID               string            `json:"id"`
		Title            string            `json:"title"`
		Category         string            `json:"category"`
		Modality         string            `json:"modality"`
		PricePerDayCents int64             `json:"pricePerDayCents"`
		Zone             map[string]string `json:"zone"`
		ApproxLocation   map[string]any    `json:"approxLocation"`
		Photos           []photo           `json:"photos"`
		Owner            map[string]any    `json:"owner"`
		Status           string            `json:"status"`
		CreatedAt        string            `json:"createdAt"`
	}
	var items []item
	for i := 0; i < 20; i++ {
		items = append(items, item{
			ID: "6afd2267e0b7532da1e7fc9a", Title: "Taladro percutor inalámbrico 20 V con dos baterías", Category: "herramientas",
			Modality: "rent", PricePerDayCents: 15000, Zone: map[string]string{"municipality": "Coyoacán", "state": "CDMX"},
			ApproxLocation: map[string]any{"lat": 19.35, "lng": -99.16},
			Photos: []photo{{PublicID: "vaiven/items/6afd2267/foto-1", URL: "https://res.cloudinary.com/demo/image/upload/s--abc--/vaiven/items/6afd2267/foto-1.jpg",
				ThumbURL: "https://res.cloudinary.com/demo/image/upload/c_fill,w_320/s--abc--/vaiven/items/6afd2267/foto-1.jpg"}},
			Owner:  map[string]any{"id": "6afd2267e0b7532da1e7fc00", "displayName": "Ana López", "ratingAverage": 4.8, "ratingCount": 12},
			Status: "active", CreatedAt: "2026-11-17T12:00:00Z",
		})
	}
	b, _ := json.Marshal(map[string]any{"items": items, "nextCursor": "eyJtIjoicmVjZW50In0"})
	return b
}

// TestCompressionReport mide tamaño y latencia con y sin gzip sobre un
// servidor real (spec 018, C-08). Solo informa; los umbrales de rendimiento
// los fija el informe. go test -run CompressionReport -v
func TestCompressionReport(t *testing.T) {
	if testing.Short() {
		t.Skip("medición")
	}
	body := realisticPage()
	handler := jsonOut(200, body, nil)
	for _, mode := range []struct {
		name string
		h    http.Handler
	}{{"sin gzip", handler}, {"con gzip", Compress(DefaultCompressConfig())(handler)}} {
		srv := httptest.NewServer(mode.h)
		client := &http.Client{Transport: &http.Transport{DisableCompression: true, MaxIdleConnsPerHost: 16}}
		const workers, perWorker = 16, 200
		lat := make([]time.Duration, 0, workers*perWorker)
		var mu sync.Mutex
		var wire int
		var wg sync.WaitGroup
		start := time.Now()
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					req, _ := http.NewRequest("GET", srv.URL+"/api/v1/items", nil)
					req.Header.Set("Accept-Encoding", "gzip")
					s := time.Now()
					res, err := client.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					n, _ := io.Copy(io.Discard, res.Body)
					_ = res.Body.Close()
					mu.Lock()
					lat = append(lat, time.Since(s))
					wire = int(n)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		total := time.Since(start)
		srv.Close()
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		t.Logf("%s: %d B por respuesta (original %d B, %.1f %% menos); %d peticiones en %v; p50 %v, p95 %v",
			mode.name, wire, len(body), 100*(1-float64(wire)/float64(len(body))), len(lat), total.Round(time.Millisecond),
			lat[len(lat)/2], lat[len(lat)*95/100])
	}
}

// BenchmarkCompress mide CPU y memoria por respuesta del middleware.
//
//	go test -run x -bench Compress -benchmem ./internal/adapter/http/middleware/
func BenchmarkCompress(b *testing.B) {
	body := realisticPage()
	for _, mode := range []struct {
		name string
		h    http.Handler
	}{{"identity", jsonOut(200, body, nil)}, {"gzip", Compress(DefaultCompressConfig())(jsonOut(200, body, nil))}} {
		b.Run(mode.name, func(b *testing.B) {
			req := httptest.NewRequest("GET", "/api/v1/items", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				mode.h.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
	}
}

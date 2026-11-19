//go:build lbproto

// Package lb prueba el prototipo LOCAL de balanceo (spec 018, propuesta no
// aprobada): Caddy delante de dos réplicas de la API con MongoDB y Redis
// compartidos (deploy/lb-local). Nunca apunta a staging ni a producción.
//
//	JWT_ACCESS_SECRET=$(openssl rand -hex 32) podman compose -f deploy/lb-local/compose.yml -p vaiven-lb up -d
//	go test -tags=lbproto -count=1 -v ./tests/lb/
package lb

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const base = "http://127.0.0.1:18080"

var replicas = []string{"vaiven-lb-api1", "vaiven-lb-api2"}

func podman(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("podman", args...).CombinedOutput() // #nosec G204 -- argumentos fijos de la prueba
	if err != nil {
		t.Fatalf("podman %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type resp struct {
	status int
	header http.Header
	body   map[string]any
}

func call(t *testing.T, method, path, reqID, token, body string, hdr map[string]string) resp {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if reqID != "" {
		req.Header.Set("X-Request-Id", reqID)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp{status: -1}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

// servedBy cuenta, en los registros de cada réplica, los ids con ese prefijo.
func servedBy(t *testing.T, prefix string) map[string]int {
	t.Helper()
	got := map[string]int{}
	for _, c := range replicas {
		got[c] = strings.Count(podman(t, "logs", c), prefix)
	}
	return got
}

// flush vacía el Redis del prototipo entre fases (límite de tasa compartido).
func flush(t *testing.T) {
	t.Helper()
	podman(t, "exec", "vaiven-lb-redis-1", "redis-cli", "FLUSHDB")
}

func waitHealthy(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if call(t, "GET", "/readyz", "", "", "", nil).status == 200 {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("el balanceador no responde")
}

func TestLoadBalancerPrototype(t *testing.T) {
	waitHealthy(t, 90*time.Second)
	flush(t)
	run := randHex(4)

	t.Run("1-2 ambas réplicas responden y el tráfico se reparte", func(t *testing.T) {
		p := "lbdist-" + run + "-"
		for i := 0; i < 40; i++ {
			if r := call(t, "GET", "/api/v1/meta", fmt.Sprintf("%s%02d", p, i), "", "", nil); r.status != 200 {
				t.Fatalf("meta = %d", r.status)
			}
		}
		got := servedBy(t, p)
		t.Logf("reparto de 40 peticiones: %v", got)
		for _, c := range replicas {
			if got[c] < 10 {
				t.Errorf("%s recibió %d de 40", c, got[c])
			}
		}
	})

	var access, refresh string
	t.Run("6 sesión y JWT válidos entre réplicas", func(t *testing.T) {
		email := "lb-" + run + "@vaiven.invalid"
		pass := "Prueba-" + randHex(8)
		r := call(t, "POST", "/api/v1/auth/register", "", "", fmt.Sprintf(`{"email":%q,"password":%q,"displayName":"Prueba LB","acceptTerms":true,"isAdult":true}`, email, pass), nil)
		if r.status != 201 {
			t.Fatalf("registro = %d %v", r.status, r.body)
		}
		tok := r.body["tokens"].(map[string]any)
		access, refresh = tok["accessToken"].(string), tok["refreshToken"].(string)
		p := "lbme-" + run + "-"
		for i := 0; i < 10; i++ {
			if r := call(t, "GET", "/api/v1/me", fmt.Sprintf("%s%02d", p, i), access, "", nil); r.status != 200 {
				t.Fatalf("/me = %d", r.status)
			}
		}
		got := servedBy(t, p)
		t.Logf("/me con el mismo token: %v", got)
		if got[replicas[0]] == 0 || got[replicas[1]] == 0 {
			t.Error("el token debe valer en las dos réplicas")
		}
		// Rotación en una réplica; reutilizar el viejo en la otra revoca la familia.
		r1 := call(t, "POST", "/api/v1/auth/refresh", "", "", fmt.Sprintf(`{"refreshToken":%q}`, refresh), nil)
		if r1.status != 200 {
			t.Fatalf("refresh = %d", r1.status)
		}
		newRefresh := r1.body["tokens"].(map[string]any)["refreshToken"].(string)
		if r := call(t, "POST", "/api/v1/auth/refresh", "", "", fmt.Sprintf(`{"refreshToken":%q}`, refresh), nil); r.status != 401 {
			t.Errorf("reutilizar el viejo = %d", r.status)
		}
		if r := call(t, "POST", "/api/v1/auth/refresh", "", "", fmt.Sprintf(`{"refreshToken":%q}`, newRefresh), nil); r.status != 401 {
			t.Errorf("la familia debe quedar revocada = %d", r.status)
		}
		// Un token falsificado se rechaza en cualquier réplica.
		if r := call(t, "GET", "/api/v1/me", "", access[:len(access)-4]+"AAAA", "", nil); r.status != 401 {
			t.Errorf("token alterado = %d", r.status)
		}
		// Limpieza: baja de la cuenta de prueba.
		login := call(t, "POST", "/api/v1/auth/login", "", "", fmt.Sprintf(`{"email":%q,"password":%q}`, email, pass), nil)
		if login.status == 200 {
			a := login.body["tokens"].(map[string]any)["accessToken"].(string)
			if r := call(t, "DELETE", "/api/v1/me", "", a, fmt.Sprintf(`{"password":%q}`, pass), nil); r.status != 204 {
				t.Errorf("baja = %d", r.status)
			}
		}
	})
	flush(t)

	t.Run("9 concurrencia sin errores", func(t *testing.T) {
		var wg sync.WaitGroup
		var mu sync.Mutex
		codes := map[int]int{}
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := call(t, "GET", "/api/v1/meta", "", "", "", nil)
				mu.Lock()
				codes[r.status]++
				mu.Unlock()
			}()
		}
		wg.Wait()
		t.Logf("100 peticiones simultáneas: %v", codes)
		if codes[200] != 100 {
			t.Errorf("códigos = %v", codes)
		}
	})
	flush(t)

	t.Run("3-4 caída y recuperación de una réplica", func(t *testing.T) {
		podman(t, "stop", "-t", "2", replicas[0])
		p := "lbdown-" + run + "-"
		fails := 0
		start := time.Now()
		for i := 0; i < 30; i++ {
			if r := call(t, "GET", "/api/v1/meta", fmt.Sprintf("%s%02d", p, i), "", "", nil); r.status != 200 {
				fails++
			}
		}
		t.Logf("con api1 detenida: 30 peticiones en %v, %d fallidas; reparto %v", time.Since(start), fails, servedBy(t, p))
		if fails != 0 {
			t.Errorf("%d peticiones fallaron con una réplica caída", fails)
		}
		podman(t, "start", replicas[0])
		recovered := false
		p2 := "lbup-" + run + "-"
		deadline := time.Now().Add(45 * time.Second)
		for i := 0; time.Now().Before(deadline) && !recovered; i++ {
			call(t, "GET", "/api/v1/meta", fmt.Sprintf("%s%03d", p2, i), "", "", nil)
			if i%10 == 9 {
				recovered = servedBy(t, p2)[replicas[0]] > 0
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Logf("tras reiniciar api1: %v", servedBy(t, p2))
		if !recovered {
			t.Error("la réplica recuperada no volvió a recibir tráfico")
		}
	})
	flush(t)

	t.Run("5-8 límite compartido en Redis y X-Forwarded-For falsificado", func(t *testing.T) {
		codes := map[int]int{}
		p := "lbrl-" + run + "-"
		for i := 0; i < 130; i++ {
			// Cada petición finge venir de otra IP: no debe servir de nada.
			r := call(t, "GET", "/api/v1/meta", fmt.Sprintf("%s%03d", p, i), "", "", map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.%d.%d", i/250, i%250+1)})
			codes[r.status]++
		}
		t.Logf("130 peticiones con X-Forwarded-For falso: %v; reparto %v", codes, servedBy(t, p))
		if codes[200] > 120 || codes[429] < 10 {
			t.Errorf("el límite de 120/min debe ser uno solo para las dos réplicas y no dejarse engañar: %v", codes)
		}
		if out := podman(t, "logs", replicas[1]) + podman(t, "logs", replicas[0]); !strings.Contains(out, "ratelimit.blocked") {
			t.Error("falta el evento ratelimit.blocked")
		}
	})
	flush(t)

	t.Run("7 compresión una sola vez a través del proxy", func(t *testing.T) {
		req, _ := http.NewRequest("GET", base+"/api/v1/categories", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		tr := &http.Transport{DisableCompression: true}
		res, err := (&http.Client{Transport: tr}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		enc := res.Header.Values("Content-Encoding")
		t.Logf("categorías: %d bytes enviados, Content-Encoding %v, Vary %q", len(raw), enc, res.Header.Get("Vary"))
		if len(enc) > 1 {
			t.Error("doble compresión")
		}
		if len(enc) == 0 && !json.Valid(bytes.TrimSpace(raw)) {
			t.Error("cuerpo sin codificar debe ser JSON")
		}
	})

	t.Run("recursos", func(t *testing.T) {
		t.Logf("consumo:\n%s", podman(t, "stats", "--no-stream", "--format", "{{.Name}} CPU {{.CPUPerc}} MEM {{.MemUsage}}"))
	})
}

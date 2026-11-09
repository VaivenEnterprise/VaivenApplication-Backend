package expo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func msgs(n int) []Message {
	out := make([]Message, n)
	for i := range out {
		out[i] = Message{To: fmt.Sprintf("ExponentPushToken[%d]", i), Title: "T", Body: "B", Data: map[string]string{"bookingId": "b1"}, Sound: "default"}
	}
	return out
}

func TestSendBuildsRequestAndParsesTickets(t *testing.T) {
	var gotAuth, gotCT string
	var got []Message
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotCT = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if r.Method != http.MethodPost {
			t.Errorf("método %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"data":[{"status":"ok","id":"x"},{"status":"error","message":"no","details":{"error":"DeviceNotRegistered"}},{"status":"error"}]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "secreto", time.Second)
	tk, err := c.Send(context.Background(), msgs(3))
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secreto" || gotCT != "application/json" || len(got) != 3 || got[0].Data["bookingId"] != "b1" {
		t.Errorf("petición: auth=%q ct=%q msgs=%v", gotAuth, gotCT, got)
	}
	if tk[0].Error != "" || tk[1].Error != DeviceNotRegistered || tk[2].Error != "unknown" || tk[1].Status != "error" {
		t.Errorf("tickets = %+v", tk)
	}
}

func TestSendWithoutAccessTokenOmitsAuthorization(t *testing.T) {
	var has bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, has = r.Header["Authorization"]
		fmt.Fprint(w, `{"data":[{"status":"ok"}]}`)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "", 0).Send(context.Background(), msgs(1)); err != nil || has {
		t.Errorf("err=%v authorization=%v", err, has)
	}
}

func TestSendEmptyAndTooBig(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "", time.Second)
	if tk, err := c.Send(context.Background(), nil); tk != nil || err != nil {
		t.Errorf("vacío = %v, %v", tk, err)
	}
	if _, err := c.Send(context.Background(), msgs(MaxBatch+1)); err == nil {
		t.Error("más de 100 mensajes debe fallar sin llamar a Expo")
	}
}

func TestSendFailures(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500":              func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "datos personales", 500) },
		"json inválido":    func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `no`) },
		"tickets de menos": func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"data":[]}`) },
	}
	for name, h := range cases {
		srv := httptest.NewServer(h)
		_, err := NewClient(srv.URL, "", time.Second).Send(context.Background(), msgs(1))
		srv.Close()
		if err == nil {
			t.Errorf("%s: debe fallar", name)
		} else if strings.Contains(err.Error(), "personales") {
			t.Errorf("%s: el error no debe incluir el cuerpo de la respuesta: %v", name, err)
		}
	}
	// Red caída.
	if _, err := NewClient("http://127.0.0.1:1", "", time.Second).Send(context.Background(), msgs(1)); err == nil {
		t.Error("red caída debe fallar")
	}
	// URL inválida.
	if _, err := NewClient("://mal", "", time.Second).Send(context.Background(), msgs(1)); err == nil {
		t.Error("URL inválida debe fallar")
	}
}

func TestSendTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	start := time.Now()
	_, err := NewClient(srv.URL, "", 50*time.Millisecond).Send(context.Background(), msgs(1))
	if err == nil || time.Since(start) > time.Second {
		t.Errorf("debe vencer por tiempo, err=%v en %v", err, time.Since(start))
	}
}

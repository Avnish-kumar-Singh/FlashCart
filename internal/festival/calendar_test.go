package festival

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCalendarFetchKeepsNationalAndRegionalHolidays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/PublicHolidays/2026/IN" {
			t.Fatalf("unexpected calendar path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"date":"2026-01-26","localName":"Republic Day","name":"Republic Day","countryCode":"IN","global":true,"counties":null,"types":["Public"]},
			{"date":"2026-11-08","localName":"Regional Holiday","name":"Regional Holiday","countryCode":"IN","global":false,"counties":["IN-DL"],"types":["Public"]},
			{"date":"2026-05-01","localName":"Observance","name":"Observance","countryCode":"IN","global":false,"counties":null,"types":["Observance"]}
		]`))
	}))
	defer server.Close()

	client := NewCalendarClient(server.URL + "/api")
	holidays, err := client.Fetch(context.Background(), "IN", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(holidays) != 2 {
		t.Fatalf("expected national and regional public holidays, got %d", len(holidays))
	}
}

func TestLocalWindowUsesLocalMidnightAndNextLocalDay(t *testing.T) {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	start, end, err := LocalWindow("2026-10-02", location)
	if err != nil {
		t.Fatal(err)
	}
	if start.Hour() != 0 || start.Location() != location || !end.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("unexpected local festival window: %s - %s", start, end)
	}
}

func TestCalendarificFetchParsesNationalAndRegionalHolidays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/holidays" || r.URL.Query().Get("country") != "IN" || r.URL.Query().Get("year") != "2026" || r.URL.Query().Get("api_key") != "test-key" {
			t.Fatalf("unexpected Calendarific request: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"meta":{"code":200},"response":{"holidays":[
			{"name":"Republic Day","date":{"iso":"2026-01-26"},"type":["National holiday"],"locations":"All"},
			{"name":"Regional Day","date":{"iso":"2026-11-08"},"type":["Local holiday"],"locations":"Delhi"},
			{"name":"Observance","date":{"iso":"2026-05-01"},"type":["Observance"],"locations":"All"}
		]}}`))
	}))
	defer server.Close()
	client := NewCalendarificClient("test-key", server.URL+"/v2")
	holidays, err := client.Fetch(context.Background(), "IN", 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(holidays) != 2 || !holidays[0].Global || len(holidays[1].Counties) != 1 {
		t.Fatalf("unexpected Calendarific holiday mapping: %+v", holidays)
	}
}

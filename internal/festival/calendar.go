package festival

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Holiday struct {
	Date      string   `json:"date"`
	LocalName string   `json:"localName"`
	Name      string   `json:"name"`
	Country   string   `json:"countryCode"`
	Global    bool     `json:"global"`
	Counties  []string `json:"counties"`
	Types     []string `json:"types"`
}

type CalendarClient struct {
	BaseURL string
	HTTP    *http.Client
}

type Provider interface {
	Fetch(context.Context, string, int) ([]Holiday, error)
}

type CalendarificClient struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

func NewCalendarificClient(apiKey, baseURL string) *CalendarificClient {
	return &CalendarificClient{APIKey: apiKey, BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *CalendarificClient) Fetch(ctx context.Context, country string, year int) ([]Holiday, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("CALENDARIFIC_API_KEY is required for India festival calendars")
	}
	u, err := url.Parse(c.BaseURL + "/holidays")
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("api_key", c.APIKey)
	query.Set("country", strings.ToUpper(country))
	query.Set("year", fmt.Sprint(year))
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Calendarific holidays: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Calendarific returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Meta struct {
			Code int `json:"code"`
		} `json:"meta"`
		Response struct {
			Holidays []struct {
				Name string `json:"name"`
				Date struct {
					ISO string `json:"iso"`
				} `json:"date"`
				Type      []string `json:"type"`
				Locations string   `json:"locations"`
			} `json:"holidays"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Calendarific response: %w", err)
	}
	if payload.Meta.Code != 200 {
		return nil, fmt.Errorf("Calendarific API reported status %d", payload.Meta.Code)
	}
	holidays := make([]Holiday, 0, len(payload.Response.Holidays))
	for _, item := range payload.Response.Holidays {
		isPublic := false
		isGlobal := false
		for _, kind := range item.Type {
			if strings.Contains(strings.ToLower(kind), "holiday") {
				isPublic = true
			}
			if strings.Contains(strings.ToLower(kind), "national") {
				isGlobal = true
			}
		}
		if !isPublic {
			continue
		}
		locations := strings.TrimSpace(item.Locations)
		var counties []string
		if locations != "" && !strings.EqualFold(locations, "All") {
			counties = strings.Split(locations, ",")
		} else {
			isGlobal = true
		}
		holidays = append(holidays, Holiday{Date: item.Date.ISO[:10], Name: item.Name, LocalName: item.Name,
			Country: strings.ToUpper(country), Global: isGlobal, Counties: counties, Types: item.Type})
	}
	return holidays, nil
}

func NewCalendarClient(baseURL string) *CalendarClient {
	return &CalendarClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *CalendarClient) Fetch(ctx context.Context, country string, year int) ([]Holiday, error) {
	url := fmt.Sprintf("%s/PublicHolidays/%d/%s", c.BaseURL, year, strings.ToUpper(country))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch public holiday calendar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("holiday calendar returned HTTP %d", resp.StatusCode)
	}
	var holidays []Holiday
	if err := json.NewDecoder(resp.Body).Decode(&holidays); err != nil {
		return nil, fmt.Errorf("decode holiday calendar: %w", err)
	}
	filtered := make([]Holiday, 0, len(holidays))
	for _, holiday := range holidays {
		if holiday.Global || len(holiday.Counties) > 0 {
			filtered = append(filtered, holiday)
		}
	}
	return filtered, nil
}

func LocalWindow(date string, location *time.Location) (time.Time, time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid holiday date %q: %w", date, err)
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1), nil
}

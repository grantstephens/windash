package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	_ "embed"

	"github.com/flosch/pongo2/v6"
	"github.com/valyala/fastjson"

	"github.com/fastly/compute-sdk-go/fsthttp"
	"github.com/fastly/compute-sdk-go/kvstore"
	"github.com/fastly/compute-sdk-go/secretstore"
)

//go:embed index.html.tmpl
var indexTemplate string

//go:embed favicon.ico
var faviconBytes []byte

// The entry point for your application.
//
// Use this function to define your main request handling logic. It could be
// used to route based on the request properties (such as method or path), send
// the request to a backend, make completely new requests, and/or generate
// synthetic responses.
const (
	secretStoreName   = "vensys-secret"
	kvStoreName       = "vensys-data"
	secretName        = "api-key"
	TID               = "277"
	backendName       = "vensys"
	backendNameCached = "vensys-cached"
	backendURLCached  = "vensys.global.ssl.fastly.net"
	backendURL        = "api.vensys.de:8443"
	powerNominal      = 2500.00 // kW
)

var apiKey string

func getKey() string {
	if apiKey == "" {
		abs, err := secretstore.Plaintext(secretStoreName, secretName)
		if err != nil {
			fmt.Println("secret not found")
		}
		apiKey = string(abs)
	}
	return apiKey
}

func main() {
	// Log service version
	fmt.Println("Service Version:", os.Getenv("FASTLY_SERVICE_VERSION"))

	fsthttp.ServeFunc(func(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
		// Filter requests that have unexpected methods.
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE" {
			w.WriteHeader(fsthttp.StatusMethodNotAllowed)
			fmt.Fprintf(w, "This method is not allowed\n")
			return
		}

		if r.URL.Path == "/" {
			index(ctx, w, r)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			favicon(ctx, w, r)
			return
		}
		if r.URL.Path == "/last30" {
			data, err := last30(ctx)
			if err != nil {
				w.WriteHeader(fsthttp.StatusInternalServerError)
				fmt.Println(err)
				return
			}
			fmt.Println(data)
			return
		}
		if r.URL.Path == "/year" {
			data, err := getYear(ctx, 2024)
			if err != nil {
				w.WriteHeader(fsthttp.StatusInternalServerError)
				fmt.Println(err)
				return
			}
			fmt.Println(data)
			return
		}
		if r.URL.Path == "/history" {
			history(ctx, w, r)
			return
		}

		// Catch all other requests and return a 404.
		w.WriteHeader(fsthttp.StatusNotFound)
		fmt.Fprintf(w, "The page you requested could not be found\n")
	})
}

func index(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
	latestPerf, age, err := getLatestPerf(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	latestMean, _, err := getLatestMean(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	// fmt.Println(latestMean)
	t, err := pongo2.FromString(indexTemplate)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	par, err := fastjson.Parse(latestPerf)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	mean, err := fastjson.Parse(latestMean)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	last30, err := last30(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	l30, err := fastjson.Parse(last30)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	var windAvgArr [30]float64
	var windMaxArr [30]float64
	var energyYieldArr [30]float64
	var availArr [30]float64
	var lowWindArr [30]float64
	var dayArr [30]string
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	for i, day := range l30.GetArray("data") {
		windAvgArr[i] = day.GetFloat64("windAvg")
		windMaxArr[i] = day.GetFloat64("windMax")
		availArr[i] = day.GetFloat64("availability")
		lowWindArr[i] = day.GetFloat64("lowWindTime") / 86400 * 100
		energyYieldArr[i] = day.GetFloat64("energyYield") / 1e3
		d := yesterday.Add(time.Duration((-30+i)*24) * time.Hour)
		dayArr[i] = d.Format("2 Jan")
	}
	// fmt.Println("powerPct", par.GetFloat64("data", "0", "powerAvg")/powerNominal*100)
	err = t.ExecuteWriter(pongo2.Context{
		"energyYield":    par.GetFloat64("data", "0", "energyYield"),
		"powerAvg":       par.GetFloat64("data", "0", "powerAvg"),
		"powerAvgPct":    par.GetFloat64("data", "0", "powerAvg") / powerNominal * 100,
		"windAvg":        par.GetFloat64("data", "0", "windAvg"),
		"windAvgArr":     windAvgArr,
		"windMaxArr":     windMaxArr,
		"energyYieldArr": energyYieldArr,
		"dayArr":         dayArr,
		"availArr":       availArr,
		"lowWindArr":     lowWindArr,
		"genSpeed":       mean.GetFloat64("data", "0", "data", "GeneratorSpeedAvg"),
		"lastUpdate":     time.Now().Add(-time.Second * time.Duration(age)).Format(time.UnixDate),
	}, w)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	// store, err := kvstore.Open(kvStoreName)
	// if err != nil {
	// 	w.WriteHeader(fsthttp.StatusInternalServerError)
	// 	fmt.Fprintln(w, err)
	// 	return
	// }

	// data, err := store.Lookup("202504")
	// if err != nil {
	// 	w.WriteHeader(fsthttp.StatusInternalServerError)
	// 	fmt.Fprintln(w, err)
	// 	return
	// }
	// var p fastjson.Parser
	// v, err := p.Parse(data.String())
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// var s string
	// da := v.GetArray("data")
	// for _, d := range da {
	// 	s += fmt.Sprintf("%s: %s\n", d.Get("date"), d.Get("energyYield"))
	// }
	// fmt.Fprint(w, s)
}

func last30(ctx context.Context) (string, error) {
	store, err := kvstore.Open(kvStoreName)
	if err != nil {
		return "", err
	}
	currentTime := time.Now()
	end := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day()-1, 0, 0, 0, 0, time.UTC)
	start := end.Add(-30 * 24 * time.Hour)
	end = end.Add(-time.Second)
	prev := start.Add(-time.Second)
	if entry, err := store.Lookup(end.Format("060102")); err == nil {
		return entry.String(), err
	}
	// Construct base URL
	baseURL := fmt.Sprintf("https://%s/api/v1.0/Customer/Performance", backendURL)

	// Create URL with query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// Add query parameters
	query := url.Values{}
	query.Add("From", fmt.Sprintf("%d", start.Unix()))
	query.Add("To", fmt.Sprintf("%d", end.Unix()))

	parsedURL.RawQuery = query.Encode()
	req, err := fsthttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("ApiKey", getKey())
	req.Header.Set("TID", TID)
	req.CacheOptions = fsthttp.CacheOptions{TTL: 10 * 60}

	resp, err := req.Send(ctx, backendName)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if store.Insert(end.Format("060102"), bytes.NewReader(data)) != nil {
		return "", err
	}
	store.Delete(prev.Format("060102"))
	return string(data), nil
}

func getYear(ctx context.Context, year int) (string, error) {
	store, err := kvstore.Open(kvStoreName)
	if err != nil {
		return "", err
	}

	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)

	if entry, err := store.Lookup(end.Format("2006")); err == nil {
		return entry.String(), err
	}
	// Construct base URL
	baseURL := fmt.Sprintf("https://%s/api/v1.0/Customer/Performance", backendURL)

	// Create URL with query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// Add query parameters
	query := url.Values{}
	query.Add("From", fmt.Sprintf("%d", start.Unix()))
	query.Add("To", fmt.Sprintf("%d", end.Unix()))

	parsedURL.RawQuery = query.Encode()
	req, err := fsthttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("ApiKey", getKey())
	req.Header.Set("TID", TID)
	req.CacheOptions = fsthttp.CacheOptions{TTL: 10 * 60}

	resp, err := req.Send(ctx, backendName)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if store.Insert(end.Format("2006"), bytes.NewReader(data)) != nil {
		return "", err
	}

	return string(data), nil
}

func history(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
	q := r.URL.Query()
	month := q.Get("month")
	if month == "" {
		w.WriteHeader(fsthttp.StatusBadRequest)
		fmt.Fprintf(w, "No month given")
		return
	}
	im, err := strconv.Atoi(month)
	if err != nil {
		w.WriteHeader(fsthttp.StatusBadRequest)
		fmt.Fprintf(w, "Bad month")
		return
	}

	store, err := kvstore.Open(kvStoreName)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintln(w, err)
		return
	}

	data, err := store.Lookup("2025" + fmt.Sprintf("%02d", im))
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintln(w, err)
		return
	}
	// w.Header().Reset(resp.Header.Clone())
	fmt.Fprint(w, data.String())
	// w.Write([]byte(data.String()))
	// io.Copy(w, data.String())
}

func getLatestPerf(ctx context.Context) (string, int, error) {
	var p string
	var a int
	baseURL := fmt.Sprintf("https://%s/api/v1.0/Customer/Performance", backendURL)

	req, err := fsthttp.NewRequest("GET", baseURL, nil)
	if err != nil {
		return p, a, err
	}

	req.Header.Set("ApiKey", getKey())
	req.Header.Set("TID", TID)
	req.CacheOptions = fsthttp.CacheOptions{TTL: 10 * 60}
	resp, err := req.Send(ctx, backendName)
	if err != nil {
		return p, a, err
	}
	if resp.StatusCode > 299 {
		return p, a, errors.New(fsthttp.StatusText(resp.StatusCode))
	}
	// a, cached := resp.Age()
	// fmt.Println(a, cached)
	// h, cached := resp.TTL()
	// fmt.Println(h, cached)
	fmt.Println("Age", resp.Header.Get("Age"))
	if hAge := resp.Header.Get("Age"); hAge != "" {
		a, err = strconv.Atoi(resp.Header.Get("Age"))
		if err != nil {
			return p, a, err
		}
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return p, a, err
	}
	return string(b), a, nil
}

func getLatestMean(ctx context.Context) (string, uint32, error) {
	var p string
	var a uint32
	baseURL := fmt.Sprintf("https://%s/api/v1.0/Customer/MeanData", backendURL)

	// Create URL with query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return p, a, err
	}

	// Add query parameters
	query := url.Values{}
	now := time.Now().Round(time.Minute * 10)
	query.Add("From", fmt.Sprintf("%d", now.Add(-time.Minute*70).Unix()))
	query.Add("To", fmt.Sprintf("%d", now.Add(-time.Hour).Unix()))
	// query.Add("Fields", *fields)
	parsedURL.RawQuery = query.Encode()

	// Create a new request
	req, err := fsthttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return p, a, err
	}

	req.Header.Set("ApiKey", getKey())
	req.Header.Set("TID", TID)
	req.CacheOptions = fsthttp.CacheOptions{TTL: 10 * 60}
	resp, err := req.Send(ctx, backendName)
	if err != nil {
		return p, a, err
	}
	if resp.StatusCode > 299 {
		return p, a, errors.New(fsthttp.StatusText(resp.StatusCode))
	}
	a, _ = resp.Age()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return p, a, err
	}
	return string(b), a, nil
}

func favicon(_ context.Context, w fsthttp.ResponseWriter, _ *fsthttp.Request) {
	io.Copy(w, bytes.NewReader(faviconBytes))
}

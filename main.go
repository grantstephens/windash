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
		if r.URL.Path == "/export/monthly" {
			exportMonthly(ctx, w, r)
			return
		}
		if r.URL.Path == "/export/yearly" {
			exportYearly(ctx, w, r)
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
		d := yesterday.Add(time.Duration((-29+i)*24) * time.Hour)
		dayArr[i] = d.Format("2 Jan")
	}

	// Get monthly data
	monthlyData, err := getLast12Months(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	monthly, err := fastjson.Parse(monthlyData)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	var monthlyLabelsArr [12]string
	var monthlyYieldArr [12]float64
	var monthlyIsCurrentArr [12]bool
	var monthlyCapacityFactorArr [12]float64
	var monthlyYoyChangeArr [12]float64
	for i, m := range monthly.GetArray("months") {
		monthlyLabelsArr[i] = string(m.GetStringBytes())
	}
	for i, y := range monthly.GetArray("energyYield") {
		monthlyYieldArr[i] = y.GetFloat64()
	}
	for i, c := range monthly.GetArray("isCurrentMonth") {
		monthlyIsCurrentArr[i] = c.GetBool()
	}
	for i, cf := range monthly.GetArray("capacityFactor") {
		monthlyCapacityFactorArr[i] = cf.GetFloat64()
	}
	for i, yoy := range monthly.GetArray("yoyChange") {
		monthlyYoyChangeArr[i] = yoy.GetFloat64()
	}

	// Get yearly data
	yearlyData, err := getYearsSince2020(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}
	yearly, err := fastjson.Parse(yearlyData)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	// Calculate dynamic array size (2022 to current year)
	yearCount := time.Now().Year() - 2022 + 1
	yearlyLabelsArr := make([]string, yearCount)
	yearlyYieldArr := make([]float64, yearCount)
	yearlyCapacityFactorArr := make([]float64, yearCount)
	yearlyYoyChangeArr := make([]float64, yearCount)
	for i, y := range yearly.GetArray("years") {
		yearlyLabelsArr[i] = string(y.GetStringBytes())
	}
	for i, y := range yearly.GetArray("energyYield") {
		yearlyYieldArr[i] = y.GetFloat64()
	}
	for i, cf := range yearly.GetArray("capacityFactor") {
		yearlyCapacityFactorArr[i] = cf.GetFloat64()
	}
	for i, yoy := range yearly.GetArray("yoyChange") {
		yearlyYoyChangeArr[i] = yoy.GetFloat64()
	}

	// Get year-to-date total
	ytdTotal, err := getYearToDateTotal(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	// Calculate YTD year-over-year change
	ytdYoyChange := 0.0
	prevYearYTD, err := getYearToDateTotalForYear(ctx, time.Now().Year()-1, int(time.Now().Month()))
	if err == nil && prevYearYTD > 0 {
		ytdYoyChange = ((ytdTotal - prevYearYTD) / prevYearYTD) * 100
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
		"monthlyLabels":         monthlyLabelsArr,
		"monthlyYield":          monthlyYieldArr,
		"monthlyIsCurrent":      monthlyIsCurrentArr,
		"monthlyCapacityFactor": monthlyCapacityFactorArr,
		"monthlyYoyChange":      monthlyYoyChangeArr,
		"yearlyLabels":          yearlyLabelsArr,
		"yearlyYield":           yearlyYieldArr,
		"yearlyCapacityFactor":  yearlyCapacityFactorArr,
		"yearlyYoyChange":       yearlyYoyChangeArr,
		"ytdTotal":              ytdTotal,
		"ytdYoyChange":          ytdYoyChange,
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
	end := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, time.UTC)
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

func getMonthlyData(ctx context.Context, year int, month int) (float64, error) {
	store, err := kvstore.Open(kvStoreName)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	isCurrentMonth := (year == currentYear && time.Month(month) == currentMonth)

	keyStr := fmt.Sprintf("monthly-%04d%02d", year, month)

	// Check cache if not current month
	if !isCurrentMonth {
		if entry, err := store.Lookup(keyStr); err == nil {
			var p fastjson.Parser
			v, err := p.Parse(entry.String())
			if err != nil {
				return 0, err
			}
			return v.GetFloat64("data", "0", "energyYield"), nil
		}
	}

	// Calculate month boundaries
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Second) // Last second of month

	// Fetch data from API
	baseURL := fmt.Sprintf("https://%s/api/v1.0/Customer/Performance", backendURL)
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return 0, err
	}

	query := url.Values{}
	query.Add("From", fmt.Sprintf("%d", start.Unix()))
	query.Add("To", fmt.Sprintf("%d", end.Unix()))
	parsedURL.RawQuery = query.Encode()

	req, err := fsthttp.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("ApiKey", getKey())
	req.Header.Set("TID", TID)
	req.CacheOptions = fsthttp.CacheOptions{TTL: 10 * 60}

	resp, err := req.Send(ctx, backendName)
	if err != nil {
		return 0, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Parse and sum energy yield
	var p fastjson.Parser
	v, err := p.Parse(string(data))
	if err != nil {
		return 0, err
	}

	totalEnergyYield := 0.0
	dataArray := v.GetArray("data")
	for _, day := range dataArray {
		totalEnergyYield += day.GetFloat64("energyYield")
	}

	// Convert from kWh to MWh
	totalEnergyYieldMWh := totalEnergyYield / 1000.0

	// Store in KV if not current month
	if !isCurrentMonth {
		storedData := fmt.Sprintf(`{"data":[{"month":"%04d%02d","energyYield":%f}]}`, year, month, totalEnergyYieldMWh)
		store.Insert(keyStr, bytes.NewReader([]byte(storedData)))
	}

	return totalEnergyYieldMWh, nil
}

func getLast12Months(ctx context.Context) (string, error) {
	now := time.Now()
	// Include current month (even if incomplete)
	year, month, _ := now.Date()
	currentMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	startMonth := currentMonth.AddDate(0, -11, 0)

	var monthLabels []string
	var energyYields []float64
	var isCurrentMonth []bool
	var capacityFactors []float64
	var yoyChanges []float64

	for i := 0; i < 12; i++ {
		targetMonth := startMonth.AddDate(0, i, 0)
		y := targetMonth.Year()
		m := int(targetMonth.Month())

		// Get monthly data
		energyYield, err := getMonthlyData(ctx, y, m)
		if err != nil {
			return "", err
		}

		// Get previous year's same month for YoY comparison
		prevYearYield, err := getMonthlyData(ctx, y-1, m)
		yoyChange := 0.0
		if err == nil && prevYearYield > 0 {
			yoyChange = ((energyYield - prevYearYield) / prevYearYield) * 100
		}

		// Check if this is the current month
		isCurrent := (y == now.Year() && time.Month(m) == now.Month())

		// Calculate capacity factor
		nextMonth := targetMonth.AddDate(0, 1, 0)
		hoursInMonth := nextMonth.Sub(targetMonth).Hours()
		theoreticalMaxMWh := (powerNominal / 1000.0) * hoursInMonth
		capacityFactor := 0.0
		if theoreticalMaxMWh > 0 {
			capacityFactor = (energyYield / theoreticalMaxMWh) * 100
		}

		// Format month label
		monthLabel := targetMonth.Format("Jan 2006")
		monthLabels = append(monthLabels, monthLabel)
		energyYields = append(energyYields, energyYield)
		isCurrentMonth = append(isCurrentMonth, isCurrent)
		capacityFactors = append(capacityFactors, capacityFactor)
		yoyChanges = append(yoyChanges, yoyChange)
	}

	// Build JSON response
	result := `{"months":[`
	for i, label := range monthLabels {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`"%s"`, label)
	}
	result += `],"energyYield":[`
	for i, yield := range energyYields {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, yield)
	}
	result += `],"isCurrentMonth":[`
	for i, isCurrent := range isCurrentMonth {
		if i > 0 {
			result += ","
		}
		if isCurrent {
			result += "true"
		} else {
			result += "false"
		}
	}
	result += `],"capacityFactor":[`
	for i, cf := range capacityFactors {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, cf)
	}
	result += `],"yoyChange":[`
	for i, yoy := range yoyChanges {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, yoy)
	}
	result += `]}`

	return result, nil
}

func getYearlyData(ctx context.Context, year int) (float64, error) {
	store, err := kvstore.Open(kvStoreName)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	currentYear := now.Year()
	isCurrentYear := (year == currentYear)

	keyStr := fmt.Sprintf("yearly-%04d", year)

	// Check cache if not current year
	if !isCurrentYear {
		if entry, err := store.Lookup(keyStr); err == nil {
			var p fastjson.Parser
			v, err := p.Parse(entry.String())
			if err != nil {
				return 0, err
			}
			return v.GetFloat64("data", "0", "energyYield"), nil
		}
	}

	// Sum monthly data for the year
	totalEnergyYield := 0.0
	for month := 1; month <= 12; month++ {
		monthlyYield, err := getMonthlyData(ctx, year, month)
		if err != nil {
			return 0, err
		}
		totalEnergyYield += monthlyYield
	}

	// Store in KV if not current year
	if !isCurrentYear {
		storedData := fmt.Sprintf(`{"data":[{"year":"%04d","energyYield":%f}]}`, year, totalEnergyYield)
		store.Insert(keyStr, bytes.NewReader([]byte(storedData)))
	}

	return totalEnergyYield, nil
}

func getYearsSince2020(ctx context.Context) (string, error) {
	now := time.Now()
	currentYear := now.Year()
	startYear := 2022

	var yearLabels []string
	var energyYields []float64
	var capacityFactors []float64
	var yoyChanges []float64

	for year := startYear; year <= currentYear; year++ {
		// Get yearly data (in MWh)
		energyYield, err := getYearlyData(ctx, year)
		if err != nil {
			return "", err
		}

		// Get previous year for YoY comparison
		prevYearYield, err := getYearlyData(ctx, year-1)
		yoyChange := 0.0
		if err == nil && prevYearYield > 0 {
			yoyChange = ((energyYield - prevYearYield) / prevYearYield) * 100
		}

		// Calculate capacity factor for the year
		startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		hoursInYear := endDate.Sub(startDate).Hours()
		theoreticalMaxMWh := (powerNominal / 1000.0) * hoursInYear
		capacityFactor := 0.0
		if theoreticalMaxMWh > 0 {
			capacityFactor = (energyYield / theoreticalMaxMWh) * 100
		}

		yearLabels = append(yearLabels, fmt.Sprintf("%d", year))
		// Convert MWh to GWh
		energyYields = append(energyYields, energyYield/1000.0)
		capacityFactors = append(capacityFactors, capacityFactor)
		yoyChanges = append(yoyChanges, yoyChange)
	}

	// Build JSON response
	result := `{"years":[`
	for i, label := range yearLabels {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`"%s"`, label)
	}
	result += `],"energyYield":[`
	for i, yield := range energyYields {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, yield)
	}
	result += `],"capacityFactor":[`
	for i, cf := range capacityFactors {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, cf)
	}
	result += `],"yoyChange":[`
	for i, yoy := range yoyChanges {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`%f`, yoy)
	}
	result += `]}`

	return result, nil
}

func getYearToDateTotal(ctx context.Context) (float64, error) {
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())

	var ytdTotal float64
	for month := 1; month <= currentMonth; month++ {
		monthlyYield, err := getMonthlyData(ctx, currentYear, month)
		if err != nil {
			return 0, err
		}
		ytdTotal += monthlyYield
	}

	return ytdTotal, nil
}

func getYearToDateTotalForYear(ctx context.Context, year int, upToMonth int) (float64, error) {
	var ytdTotal float64
	for month := 1; month <= upToMonth; month++ {
		monthlyYield, err := getMonthlyData(ctx, year, month)
		if err != nil {
			return 0, err
		}
		ytdTotal += monthlyYield
	}
	return ytdTotal, nil
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

func exportMonthly(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
	format := r.URL.Query().Get("format")

	monthlyData, err := getLast12Months(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintf(w, "Error fetching monthly data: %v\n", err)
		return
	}

	monthly, err := fastjson.Parse(monthlyData)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintf(w, "Error parsing data: %v\n", err)
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=monthly_production.csv")

		// Write CSV header
		fmt.Fprintln(w, "Month,Energy (MWh),Capacity Factor (%),YoY Change (%)")

		// Write data rows
		for i, m := range monthly.GetArray("months") {
			month := string(m.GetStringBytes())
			yield := monthly.GetArray("energyYield")[i].GetFloat64()
			cf := monthly.GetArray("capacityFactor")[i].GetFloat64()
			yoy := monthly.GetArray("yoyChange")[i].GetFloat64()
			fmt.Fprintf(w, "%s,%.2f,%.2f,%.2f\n", month, yield, cf, yoy)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=monthly_production.json")
		fmt.Fprint(w, monthlyData)
	}
}

func exportYearly(ctx context.Context, w fsthttp.ResponseWriter, r *fsthttp.Request) {
	format := r.URL.Query().Get("format")

	yearlyData, err := getYearsSince2020(ctx)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintf(w, "Error fetching yearly data: %v\n", err)
		return
	}

	yearly, err := fastjson.Parse(yearlyData)
	if err != nil {
		w.WriteHeader(fsthttp.StatusInternalServerError)
		fmt.Fprintf(w, "Error parsing data: %v\n", err)
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=yearly_production.csv")

		// Write CSV header
		fmt.Fprintln(w, "Year,Energy (GWh),Capacity Factor (%),YoY Change (%)")

		// Write data rows
		for i, y := range yearly.GetArray("years") {
			year := string(y.GetStringBytes())
			yield := yearly.GetArray("energyYield")[i].GetFloat64()
			cf := yearly.GetArray("capacityFactor")[i].GetFloat64()
			yoy := yearly.GetArray("yoyChange")[i].GetFloat64()
			fmt.Fprintf(w, "%s,%.2f,%.2f,%.2f\n", year, yield, cf, yoy)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=yearly_production.json")
		fmt.Fprint(w, yearlyData)
	}
}

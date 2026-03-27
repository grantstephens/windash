package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/flosch/pongo2/v6"
)

func main() {

	ctx := pongo2.Context{
		"powerAvg":              850.0,
		"powerAvgPct":           56.7,
		"powerAvgSpinDuration":  4.6,
		"windAvg":               8.42,
		"energyYield":           12450.0,
		"ytdTotal":              1823.5,
		"ytdYoyChange":          12.3,
		"lastUpdate":            "Thu Mar 27 14:30:00 GMT 2026",
		"version":               "preview",
		"dayArr":                []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27", "28", "29", "30"},
		"windAvgArr":            []float64{6.2, 7.1, 5.8, 8.3, 9.1, 7.5, 6.8, 8.9, 10.2, 7.4, 6.1, 8.7, 9.5, 7.8, 6.3, 8.1, 9.8, 7.2, 6.5, 8.4, 9.3, 7.6, 6.9, 8.8, 10.1, 7.3, 6.0, 8.6, 9.4, 7.7},
		"windMaxArr":            []float64{12.1, 14.3, 11.2, 15.6, 16.8, 13.9, 12.5, 15.2, 18.1, 13.5, 11.8, 15.9, 17.2, 14.1, 11.5, 14.8, 17.6, 13.2, 11.9, 15.3, 16.9, 14.0, 12.6, 15.8, 18.3, 13.4, 11.1, 15.7, 17.0, 14.2},
		"energyYieldArr":        []float64{320, 410, 280, 520, 580, 430, 350, 540, 650, 400, 290, 530, 600, 440, 310, 500, 620, 390, 330, 510, 590, 420, 360, 550, 640, 380, 270, 520, 610, 450},
		"availArr":              []float64{98, 99, 97, 100, 100, 99, 98, 100, 100, 99, 97, 100, 100, 99, 98, 100, 100, 99, 98, 100, 100, 99, 98, 100, 100, 99, 97, 100, 100, 99},
		"lowWindArr":            []float64{2, 1, 3, 0, 0, 1, 2, 0, 0, 1, 3, 0, 0, 1, 2, 0, 0, 1, 2, 0, 0, 1, 2, 0, 0, 1, 3, 0, 0, 1},
		"monthlyLabels":         []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		"monthlyYield":          []float64{680, 590, 520, 450, 380, 320, 290, 340, 410, 510, 620, 700},
		"monthlyIsCurrent":      []bool{false, false, true, false, false, false, false, false, false, false, false, false},
		"monthlyCapacityFactor": []float64{38.2, 36.7, 29.2, 26.1, 21.3, 18.6, 16.3, 19.1, 23.8, 28.6, 36.0, 39.3},
		"monthlyYoyChange":      []float64{5.2, -3.1, 8.4, 2.1, -1.5, 4.3, -2.8, 6.1, 3.7, -0.9, 7.2, 4.5},
		"yearlyLabels":          []string{"2020", "2021", "2022", "2023", "2024", "2025", "2026"},
		"yearlyYield":           []float64{4200, 4850, 5100, 4750, 5300, 5500, 1823},
		"yearlyCapacityFactor":  []float64{24.1, 27.8, 29.2, 27.2, 30.4, 31.5, 29.8},
		"yearlyYoyChange":       []float64{0, 15.5, 5.2, -6.9, 11.6, 3.8, 12.3},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tpl, err := pongo2.FromFile("index.html.tmpl")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err = tpl.ExecuteWriter(ctx, w)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
	})

	http.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "favicon.svg")
	})

	fmt.Println("Preview server running at http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

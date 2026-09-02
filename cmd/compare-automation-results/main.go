package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"api-automation/internal/resultcompare"
)

func main() {
	gioPath := flag.String("gio", "", "path to the Gio automation JSONL result store")
	wailsPath := flag.String("wails", "", "path to the Wails automation JSONL result store")
	flag.Parse()
	if *gioPath == "" || *wailsPath == "" {
		fmt.Fprintln(os.Stderr, "both -gio and -wails result paths are required")
		flag.Usage()
		os.Exit(2)
	}
	report, err := resultcompare.Compare(*gioPath, *wailsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if !report.Equivalent {
		os.Exit(1)
	}
}

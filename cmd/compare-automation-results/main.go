package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"api-automation/internal/resultcompare"
)

func main() {
	baselinePath := flag.String("baseline", "", "path to the baseline automation JSONL result store")
	candidatePath := flag.String("candidate", "", "path to the candidate automation JSONL result store")
	flag.Parse()
	if *baselinePath == "" || *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "both -baseline and -candidate result paths are required")
		flag.Usage()
		os.Exit(2)
	}
	report, err := resultcompare.Compare(*baselinePath, *candidatePath)
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

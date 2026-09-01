package pagevalues

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const DefaultExpansionLimit = 10_000

var rangePattern = regexp.MustCompile(`^([0-9]+)\s*(?:-|(?i:to))\s*([0-9]+)$`)

type Selection struct {
	Pages      []int
	Normalized string
	MaxPage    int
}

// Parse accepts the same list/range forms as Copies, but treats the expanded
// values as one custom page selection rather than separate automation cases.
func Parse(input string, expansionLimit int) (Selection, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Selection{}, fmt.Errorf("page range is required")
	}
	if expansionLimit < 1 {
		expansionLimit = DefaultExpansionLimit
	}
	seen := make(map[int]struct{})
	for _, rawToken := range strings.Split(input, ",") {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			return Selection{}, fmt.Errorf("page range contains an empty value")
		}
		if match := rangePattern.FindStringSubmatch(token); len(match) == 3 {
			start, _ := strconv.Atoi(match[1])
			end, _ := strconv.Atoi(match[2])
			if start < 1 || end < 1 {
				return Selection{}, fmt.Errorf("page numbers must be at least 1")
			}
			if start > end {
				return Selection{}, fmt.Errorf("page range %q must be ascending", token)
			}
			for page := start; page <= end; page++ {
				seen[page] = struct{}{}
				if len(seen) > expansionLimit {
					return Selection{}, fmt.Errorf("page range expands beyond the %d-page safety limit", expansionLimit)
				}
			}
			continue
		}
		page, err := strconv.Atoi(token)
		if err != nil || page < 1 {
			return Selection{}, fmt.Errorf("invalid page value %q; use forms such as 1,3,5-8 or 5 to 8", token)
		}
		seen[page] = struct{}{}
		if len(seen) > expansionLimit {
			return Selection{}, fmt.Errorf("page range expands beyond the %d-page safety limit", expansionLimit)
		}
	}
	pages := make([]int, 0, len(seen))
	for page := range seen {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	if len(pages) == 0 {
		return Selection{}, fmt.Errorf("page range is required")
	}
	return Selection{Pages: pages, Normalized: compact(pages), MaxPage: pages[len(pages)-1]}, nil
}

func (s Selection) ValidatePageCount(pageCount int) error {
	if pageCount < 1 {
		return fmt.Errorf("the imported file did not report a usable page count")
	}
	if s.MaxPage > pageCount {
		return fmt.Errorf("requested page %d exceeds the imported file's %d page(s)", s.MaxPage, pageCount)
	}
	return nil
}

func Equivalent(left, right string) bool {
	leftSelection, leftErr := Parse(left, DefaultExpansionLimit)
	rightSelection, rightErr := Parse(right, DefaultExpansionLimit)
	return leftErr == nil && rightErr == nil && leftSelection.Normalized == rightSelection.Normalized
}

func compact(pages []int) string {
	parts := make([]string, 0, len(pages))
	for index := 0; index < len(pages); {
		end := index
		for end+1 < len(pages) && pages[end+1] == pages[end]+1 {
			end++
		}
		if end == index {
			parts = append(parts, strconv.Itoa(pages[index]))
		} else {
			parts = append(parts, strconv.Itoa(pages[index])+"-"+strconv.Itoa(pages[end]))
		}
		index = end + 1
	}
	return strings.Join(parts, ",")
}

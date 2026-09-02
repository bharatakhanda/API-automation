package application

const (
	// Fiery job-property wire identifiers. Keep these values exact; the API is
	// case-sensitive and EFPageRange is the authoritative custom-range carrier.
	PageRangeOptionID       = "EFPageRange"
	PageRangeLegacyDataID   = "DPP_PAGE_RANGE"
	PageRangeRangeValue     = "Range1"
	PageRangeInternalPrefix = "__API_AUTOMATION_CUSTOM_PAGE_RANGE__:"
	OutputProfileOptionID   = "EFOutProfile"
	CopiesOptionID          = "num copies"

	DefaultCaseLimit   = 100
	MaximumCaseLimit   = 10_000
	DefaultWorkerCount = 1
	MaximumWorkerCount = 10
)

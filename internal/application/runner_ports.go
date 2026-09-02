package application

import (
	"context"

	"api-automation/internal/fiery"
	"api-automation/internal/reportxlsx"
)

// AutomationConnector establishes one authenticated, run-scoped client. It is
// implemented by frontend adapters so credentials never enter event DTOs.
type AutomationConnector interface {
	Connect(context.Context) (AutomationClient, ConnectionInfo, error)
}

type ConnectionInfo struct {
	SessionLoginPath string
}

// AutomationClient is the narrow Fiery port consumed by the runner. A concrete
// adapter binds fiery.Client and fiery.Session once at connection time.
type AutomationClient interface {
	ImportJobToQueue(context.Context, string, string) (fiery.ImportResult, error)
	GetJobAttributes(context.Context, string) (map[string]string, error)
	ApplyServerPreset(context.Context, string, string) error
	CheckJobConstraints(context.Context, string, map[string]string) (fiery.ConstraintCheck, error)
	UpdateJobAttributes(context.Context, string, map[string]string) error
	DeleteJob(context.Context, string) error
	CancelJob(context.Context, string) error
	JobAction(context.Context, string, string) error
	GetRawJobResponses(context.Context, string) []fiery.JobRawResponse
}

// ResultRecorder retains complete results independently of bounded frontend
// history. reportxlsx.ResultStore satisfies this interface.
type ResultRecorder interface {
	Append(reportxlsx.Result) error
	Close() error
}

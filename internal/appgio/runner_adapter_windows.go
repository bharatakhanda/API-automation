//go:build windows

package appgio

import (
	"context"
	"fmt"

	"api-automation/internal/application"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
)

type fieryAutomationConnector struct {
	server model.ServerConnection
}

func (connector fieryAutomationConnector) Connect(ctx context.Context) (application.AutomationClient, application.ConnectionInfo, error) {
	client, err := fiery.New(fiery.Config{
		ServerIP:    connector.server.IPAddress,
		SecretKey:   connector.server.SecretKey,
		Password:    connector.server.Password,
		InsecureTLS: true,
	})
	if err != nil {
		return nil, application.ConnectionInfo{}, fmt.Errorf("server configuration invalid: %w", err)
	}
	session, err := client.Login(ctx)
	if err != nil {
		return nil, application.ConnectionInfo{}, fmt.Errorf("login failed: %w", err)
	}
	return newFieryAutomationClient(client, session), application.ConnectionInfo{SessionLoginPath: session.LoginPath}, nil
}

type fieryAutomationClient struct {
	client  *fiery.Client
	session fiery.Session
}

func newFieryAutomationClient(client *fiery.Client, session fiery.Session) *fieryAutomationClient {
	return &fieryAutomationClient{client: client, session: session}
}

func (client *fieryAutomationClient) ImportJobToQueue(ctx context.Context, file, queue string) (fiery.ImportResult, error) {
	return client.client.ImportJobToQueue(ctx, client.session, file, queue)
}

func (client *fieryAutomationClient) GetJobAttributes(ctx context.Context, jobID string) (map[string]string, error) {
	return client.client.GetJobAttributes(ctx, client.session, jobID)
}

func (client *fieryAutomationClient) ApplyServerPreset(ctx context.Context, jobID, presetID string) error {
	return client.client.ApplyServerPreset(ctx, client.session, jobID, presetID)
}

func (client *fieryAutomationClient) CheckJobConstraints(ctx context.Context, jobID string, attributes map[string]string) (fiery.ConstraintCheck, error) {
	return client.client.CheckJobConstraints(ctx, client.session, jobID, attributes)
}

func (client *fieryAutomationClient) UpdateJobAttributes(ctx context.Context, jobID string, attributes map[string]string) error {
	return client.client.UpdateJobAttributes(ctx, client.session, jobID, attributes)
}

func (client *fieryAutomationClient) DeleteJob(ctx context.Context, jobID string) error {
	return client.client.DeleteJob(ctx, client.session, jobID)
}

func (client *fieryAutomationClient) CancelJob(ctx context.Context, jobID string) error {
	return client.client.CancelJob(ctx, client.session, jobID)
}

func (client *fieryAutomationClient) JobAction(ctx context.Context, jobID, action string) error {
	return client.client.JobAction(ctx, client.session, jobID, action)
}

func (client *fieryAutomationClient) GetRawJobResponses(ctx context.Context, jobID string) []fiery.JobRawResponse {
	return client.client.GetRawJobResponses(ctx, client.session, jobID)
}

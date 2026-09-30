package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azlogs"
)

// Row is one result row, keyed by column name.
type Row map[string]any

// Querier runs one KQL query against a Log Analytics workspace. from and to
// bound the query's timespan; nil bounds mean no timespan (the retention probe
// must see the whole table). It is an interface so the engine is tested with
// recorded rows and never touches the network.
type Querier interface {
	Query(ctx context.Context, workspaceID, kql string, from, to *time.Time) ([]Row, error)
}

// LogsQuerier is the Querier over the Azure Monitor Logs query API. The client
// requests a token for the Log Analytics audience (api.loganalytics.io) from
// the same delegated credential that reads Microsoft Graph.
type LogsQuerier struct {
	client *azlogs.Client
}

// NewLogsQuerier builds a LogsQuerier. It performs no network call: the token
// is requested with the first query.
func NewLogsQuerier(cred azcore.TokenCredential) (*LogsQuerier, error) {
	client, err := azlogs.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Log Analytics client: %w", err)
	}
	return &LogsQuerier{client: client}, nil
}

// Query runs kql against the workspace and returns the rows of its primary
// result table. A partial result (the service reports an error alongside
// rows) is an error: a truncated answer would read as "no event".
func (q *LogsQuerier) Query(ctx context.Context, workspaceID, kql string, from, to *time.Time) ([]Row, error) {
	body := azlogs.QueryBody{Query: &kql}
	if from != nil && to != nil {
		span := azlogs.NewTimeInterval(from.UTC(), to.UTC())
		body.Timespan = &span
	}
	resp, err := q.client.QueryWorkspace(ctx, workspaceID, body, nil)
	if err != nil {
		return nil, fmt.Errorf("log analytics query failed: %w", err)
	}
	return rowsFromResults(resp.QueryResults)
}

// rowsFromResults converts the primary result table into rows keyed by column
// name.
func rowsFromResults(res azlogs.QueryResults) ([]Row, error) {
	if res.Error != nil {
		return nil, fmt.Errorf("log analytics returned a partial result: %w", res.Error)
	}
	if len(res.Tables) == 0 {
		return nil, errors.New("log analytics returned no result table")
	}
	table := res.Tables[0]
	rows := make([]Row, 0, len(table.Rows))
	for _, raw := range table.Rows {
		row := Row{}
		for i, col := range table.Columns {
			if col.Name == nil || i >= len(raw) {
				continue
			}
			row[*col.Name] = raw[i]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

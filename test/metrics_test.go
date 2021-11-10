package test

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/prometheus/common/model"
)

type MonitoringSetType int

const (
	MonitoringSmallset MonitoringSetType = iota
	MonitoringLargeset
)

type QueryMetricsResult struct {
	Data struct {
		Result model.Vector `json:"result"`
	} `json:"data"`
}

// queryMetrics queries monitoring metrics by PromQL.
func queryMetrics(settype MonitoringSetType, promql string) (*QueryMetricsResult, error) {
	var endpoint string
	switch settype {
	case MonitoringSmallset:
		endpoint = "http://vmsingle-vmsingle-smallset.monitoring.svc:8429"
	case MonitoringLargeset:
		endpoint = "http://vmselect-vmcluster-largeset.monitoring.svc:8481/select/0/prometheus"
	default:
		return nil, fmt.Errorf("invalid settype %d", int(settype))
	}
	querystr := url.QueryEscape(promql)
	stdout, stderr, err := ExecAt(boot0, "curl", "-sf", endpoint+"/api/v1/query?query="+querystr)
	if err != nil {
		return nil, fmt.Errorf("stderr=%s: %w", string(stderr), err)
	}

	result := QueryMetricsResult{}
	err = json.Unmarshal(stdout, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Package workflows parses Actions YAML into a needs DAG.
package workflows

import (
	"fmt"

	"github.com/ncdlabs/gitea-lens/internal/models"
	"gopkg.in/yaml.v3"
)

type workflowFile struct {
	Name string                 `yaml:"name"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name  string      `yaml:"name"`
	Needs interface{} `yaml:"needs"`
}

// ParseNeedsDAG builds workflow nodes from Actions YAML.
func ParseNeedsDAG(yamlBytes []byte) ([]models.WorkflowNode, error) {
	var wf workflowFile
	if err := yaml.Unmarshal(yamlBytes, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}
	nodes := make([]models.WorkflowNode, 0, len(wf.Jobs))
	known := map[string]struct{}{}
	for key := range wf.Jobs {
		known[key] = struct{}{}
	}
	for key, job := range wf.Jobs {
		name := job.Name
		if name == "" {
			name = key
		}
		needs, raw := parseNeeds(job.Needs)
		unknown := false
		for _, n := range needs {
			if _, ok := known[n]; !ok {
				unknown = true
			}
		}
		nodes = append(nodes, models.WorkflowNode{
			JobKey:      key,
			Name:        name,
			Needs:       needs,
			RawNeeds:    raw,
			UnknownDeps: unknown,
		})
	}
	return nodes, nil
}

func parseNeeds(v interface{}) ([]string, string) {
	if v == nil {
		return nil, ""
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, ""
		}
		return []string{t}, t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, fmt.Sprintf("%v", t)
	default:
		return nil, fmt.Sprintf("%v", v)
	}
}

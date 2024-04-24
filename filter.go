package opslevel_k8s_controller

import (
	"encoding/json"
	"strconv"

	opslevel_jq_parser "github.com/opslevel/opslevel-jq-parser/v2024"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type K8SResource struct {
	Metadata v1.ObjectMeta `json:"metadata"`
}

type K8SFilter struct {
	namespaces []string
	parser     *opslevel_jq_parser.JQArrayParser
}

func NewK8SFilter(selector K8SSelector) *K8SFilter {
	return &K8SFilter{
		namespaces: selector.Namespaces,
		parser:     opslevel_jq_parser.NewJQArrayParser(selector.Excludes),
	}
}

func (f *K8SFilter) MatchesNamespace(data any) bool {
	j, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if len(f.namespaces) <= 0 {
		return true
	}
	var res K8SResource
	if err = json.Unmarshal(j, &res); err == nil {
		for _, namespace := range f.namespaces {
			if res.Metadata.Namespace == namespace {
				return true
			}
		}
	}
	return false
}

func (f *K8SFilter) MatchesFilter(data any) bool {
	j, err := json.Marshal(data)
	if err != nil {
		return false
	}

	// TODO: handle error
	results, _ := f.parser.Run(string(j))
	return anyIsTrue(results)
}

func anyIsTrue(results []string) bool {
	for _, result := range results {
		boolValue, err := strconv.ParseBool(result)
		if err != nil {
			return false // TODO: is this a good idea?
		}
		if boolValue {
			return true
		}
	}
	return false
}

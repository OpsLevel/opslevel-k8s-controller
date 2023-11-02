package opslevel_k8s_controller

import (
	"encoding/json"
	"github.com/opslevel/opslevel-jq-parser/v2023"
	"github.com/rs/zerolog/log"
	"strconv"
)

type K8SFilter struct {
	parser *opslevel_jq_parser.JQArrayParser
}

func NewK8SFilter(selector K8SSelector) *K8SFilter {
	return &K8SFilter{
		parser: opslevel_jq_parser.NewJQArrayParser(selector.Excludes),
	}
}

func (f *K8SFilter) Matches(data any) bool {
	j, err := json.Marshal(data)
	if err != nil {
		return false
	}
	results, err := f.parser.Run(string(j))
	log.Warn().Msgf("%+v\n", results)
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

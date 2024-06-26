package opslevel_k8s_controller_test

import (
	"encoding/json"
	"testing"

	opslevel_k8s_controller "github.com/opslevel/opslevel-k8s-controller/v2024"
	"github.com/rocktavious/autopilot/v2023"
)

var data = `{
	"apiVersion": "apps/v1",
	"kind": "Deployment",
	"metadata": {
		"annotations": {
			"kots.io/app-slug": "opslevel",
			"opslevel.com/exclude": "true"
		},
		"name": "default",
		"namespace": "default"
	}
}`

func TestFilter(t *testing.T) {
	selector1 := opslevel_k8s_controller.K8SSelector{
		Excludes: []string{
			`.metadata.labels.test == "true"`,
			`.metadata.annotations.test == "true"`,
			`.metadata.namespace == "default"`,
		},
	}
	filter1 := opslevel_k8s_controller.NewK8SFilter(selector1)
	selector2 := opslevel_k8s_controller.K8SSelector{
		Excludes: []string{
			`.metadata.labels.test == "true"`,
			`.metadata.namespace == "test"`,
		},
	}
	filter2 := opslevel_k8s_controller.NewK8SFilter(selector2)
	selector3 := opslevel_k8s_controller.K8SSelector{
		Excludes: []string{
			`.metadata.annotations."opslevel.com/exclude"`,
			`.metadata.namespace == "test"`,
		},
	}
	filter3 := opslevel_k8s_controller.NewK8SFilter(selector3)
	selector4 := opslevel_k8s_controller.K8SSelector{
		Excludes: []string{
			`.metadata.annotations`,
		},
	}
	filter4 := opslevel_k8s_controller.NewK8SFilter(selector4)
	selector5 := opslevel_k8s_controller.K8SSelector{
		Namespaces: []string{
			`default`,
		},
	}
	filter5 := opslevel_k8s_controller.NewK8SFilter(selector5)
	selector6 := opslevel_k8s_controller.K8SSelector{
		Namespaces: []string{
			`kube-system`,
		},
	}
	filter6 := opslevel_k8s_controller.NewK8SFilter(selector6)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		panic(err)
	}

	// Act
	matches1 := filter1.MatchesFilter(parsed)
	matches2 := filter2.MatchesFilter(parsed)
	matches3 := filter3.MatchesFilter(parsed)
	matches4 := filter4.MatchesFilter(parsed)
	matches5 := filter4.MatchesFilter(make(chan int))
	matches6 := filter5.MatchesNamespace(parsed)
	matches7 := filter6.MatchesNamespace(parsed)

	// Assert
	autopilot.Equals(t, true, matches1)
	autopilot.Equals(t, false, matches2)
	autopilot.Equals(t, true, matches3)
	autopilot.Equals(t, false, matches4)
	autopilot.Equals(t, false, matches5)
	autopilot.Equals(t, true, matches6)
	autopilot.Equals(t, false, matches7)
}

# opslevel-k8s-controller
A utility library for easily making and running k8s controllers

# Installation

```bash
go get github.com/opslevel/opslevel-k8s-controller/v2023
```

Then to create a k8s controller you can simply do

```go
selector := opslevel_k8s_controller.K8SSelector{
    ApiVersion: "apps/v1",
    Kind: "Deployment",
    Excludes: []string{`.metadata.namespace == "kube-system"`}
}
resync := time.Hour*24
batch := 500
runOnce := false
controller, err := opslevel_k8s_controller.NewK8SController(selector, resync, batch, runOnce)
if err != nil {
    //... Handle error ...
}
callback := func(items []interface{}) {
    for _, item := range items {
        // ... Process K8S Resource ...
    }
}
controller.OnAdd = callback
controller.OnUpdate = callback
controller.Start()
```

Because of the way the selector works you can easily target any k8s resource in your cluster and you have the power of JQ
to exclude resources that might match the expression.
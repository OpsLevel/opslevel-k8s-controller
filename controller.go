package opslevel_k8s_controller

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type K8SControllerHandler func(interface{})

func nullKubernetesControllerHandler(item interface{}) {}

type K8SController struct {
	id       string
	factory  dynamicinformer.DynamicSharedInformerFactory
	queue    *workqueue.Type
	informer cache.SharedIndexInformer
	filter   *K8SFilter
	OnAdd    K8SControllerHandler
	OnUpdate K8SControllerHandler
	OnDelete K8SControllerHandler
}

type K8SControllerEventType string

const (
	ControllerEventTypeCreate K8SControllerEventType = "create"
	ControllerEventTypeUpdate K8SControllerEventType = "update"
	ControllerEventTypeDelete K8SControllerEventType = "delete"
)

type K8SEvent struct {
	Key  string
	Type K8SControllerEventType
}

func (c *K8SController) mainloop(item interface{}) {
	indexer := c.informer.GetIndexer()

	event := item.(K8SEvent)
	obj, exists, err := indexer.GetByKey(event.Key)
	if err != nil {
		log.Warn().Msgf("error fetching object with key %s from informer cache: %v", event.Key, err)
		return
	}
	if !exists {
		return
	}
	if c.filter.Matches(obj) {
		log.Debug().Msgf("object with key %s skipped because it matches filter", event.Key)
		return
	}
	switch event.Type {
	case ControllerEventTypeCreate:
		c.OnAdd(obj)
	case ControllerEventTypeUpdate:
		c.OnUpdate(obj)
	case ControllerEventTypeDelete:
		c.OnDelete(obj)
	}
	c.queue.Done(item)
}

// Start - starts the informer faktory and sync's the data.
// The wait group passed in is used to track when the informer has gone
// through 1 full loop and syncronized all the k8s data 1 time
func (c *K8SController) Start(wg *sync.WaitGroup) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()
	if wg != nil {
		wg.Add(1)
	}
	c.factory.Start(nil) // Starts all informers

	for _, ready := range c.factory.WaitForCacheSync(nil) {
		if !ready {
			runtime.HandleError(fmt.Errorf("[%s] Timed out waiting for caches to sync", c.id))
			return
		}
		log.Info().Msgf("[%s] Informer is ready and synced", c.id)
	}
	go func() {
		var hasLoopedOnce bool
		for {
			item, quit := c.queue.Get()
			c.mainloop(item)
			if !hasLoopedOnce {
				wg.Done()
				hasLoopedOnce = true
			}
			if quit {
				return
			}
		}
	}()
}

func NewK8SController(selector K8SSelector, resyncInterval time.Duration) (*K8SController, error) {
	k8sClient, err := NewK8SClient()
	if err != nil {
		return nil, err
	}
	gvr, err := k8sClient.GetGVR(selector)
	if err != nil {
		return nil, err
	}

	queue := workqueue.New()
	filter := NewK8SFilter(selector)
	factory := k8sClient.GetInformerFactory(resyncInterval)
	informer := factory.ForResource(*gvr).Informer()
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			queue.Add(K8SEvent{
				Key:  key,
				Type: ControllerEventTypeCreate,
			})
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(old)
			if err != nil {
				return
			}
			queue.Add(K8SEvent{
				Key:  key,
				Type: ControllerEventTypeUpdate,
			})
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			queue.Add(K8SEvent{
				Key:  key,
				Type: ControllerEventTypeDelete,
			})
		},
	})
	return &K8SController{
		id:       fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource),
		queue:    queue,
		factory:  factory,
		informer: informer,
		filter:   filter,
		OnAdd:    nullKubernetesControllerHandler,
		OnUpdate: nullKubernetesControllerHandler,
		OnDelete: nullKubernetesControllerHandler,
	}, err
}

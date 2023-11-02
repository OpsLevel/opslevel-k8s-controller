package opslevel_k8s_controller

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type K8SControllerHandler func([]interface{})

type K8SController struct {
	id       string
	factory  dynamicinformer.DynamicSharedInformerFactory
	queue    *workqueue.Type
	informer cache.SharedIndexInformer
	filter   *K8SFilter
	maxBatch int
	runOnce  bool
	Channel  chan struct{}
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

func (c *K8SController) getLength() int {
	current := c.queue.Len()
	if current < c.maxBatch {
		return current
	} else {
		return c.maxBatch
	}
}

func (c *K8SController) mainloop() {
	var matchType K8SControllerEventType
	items := make([]interface{}, 0)
	length := c.getLength()
	for i := 0; i < length; i++ {
		item, quit := c.queue.Get()
		if quit {
			log.Warn().Msg("k8s queue is shut down")
			return
		}
		event := item.(K8SEvent)
		if i == 0 {
			// First item determines what event type we process
			matchType = event.Type
		} else {
			if event.Type != matchType {
				c.queue.Add(item)
				c.queue.Done(item)
				continue
			}
		}
		obj, exists, getErr := c.informer.GetIndexer().GetByKey(event.Key)
		if getErr != nil {
			log.Warn().Msgf("error fetching object with key %s from informer cache: %v", event.Key, getErr)
			c.queue.Done(item)
			return
		}
		if !exists {
			if matchType != ControllerEventTypeDelete {
				log.Warn().Msgf("object with key %s doesn't exist in informer cache", event.Key)
			}
			c.queue.Done(item)
			return
		}
		c.queue.Done(item)
		items = append(items, obj)
	}
	switch matchType {
	case ControllerEventTypeCreate:
		c.OnAdd(items)
	case ControllerEventTypeUpdate:
		c.OnUpdate(items)
	case ControllerEventTypeDelete:
		c.OnDelete(items)
	}
}

func (c *K8SController) Start() {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()
	c.factory.Start(c.Channel) // Starts all informers

	for _, ready := range c.factory.WaitForCacheSync(c.Channel) {
		if !ready {
			runtime.HandleError(fmt.Errorf("[%s] Timed out waiting for caches to sync", c.id))
			return
		}
		log.Info().Msgf("[%s] Informer is ready and synced", c.id)
	}

	if c.runOnce {
		c.mainloop()
		log.Info().Msgf("[%s] Finished", c.id)
	} else {
		go wait.Until(c.mainloop, time.Second, c.Channel)
		<-c.Channel
	}
}

func nullKubernetesControllerHandler(items []interface{}) {}

func NewK8SController(selector K8SSelector, resyncInterval time.Duration, maxBatch int, runOnce bool) (*K8SController, error) {
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
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
		maxBatch: maxBatch,
		runOnce:  runOnce,
		Channel:  make(chan struct{}),
		OnAdd:    nullKubernetesControllerHandler,
		OnUpdate: nullKubernetesControllerHandler,
		OnDelete: nullKubernetesControllerHandler,
	}, nil
}

package opslevel_k8s_controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	queue "github.com/opslevel/opslevel-k8s-controller/v2024/queue.go"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type K8SControllerHandler func(interface{})

func nullKubernetesControllerHandler(item interface{}) {}

type K8SController struct {
	id       string
	factory  dynamicinformer.DynamicSharedInformerFactory
	queue    *queue.Queue
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
	log.Debug().Str("queue_addr", fmt.Sprintf("%p", &c.queue)).Int("queue_len", c.queue.Len()).Msg("mainloop: running from top")
	var (
		indexer cache.Indexer = c.informer.GetIndexer()
		event   K8SEvent
	)

	if _, ok := item.(K8SEvent); !ok {
		log.Warn().Msgf("mainloop: cannot create K8SEvent from unknown interface '%T'", item)
		return
	}
	event = item.(K8SEvent)
	obj, exists, err := indexer.GetByKey(event.Key)
	if err != nil {
		log.Warn().Msgf("error fetching object with key '%s' from informer cache: '%v'", event.Key, err)
		return
	}
	if !exists {
		log.Debug().Msgf("object with key '%s' skipped because it was not found", event.Key)
		return
	}
	if c.filter.Matches(obj) {
		log.Debug().Msgf("object with key '%s' skipped because it matches filter", event.Key)
		return
	}
	switch event.Type {
	case ControllerEventTypeCreate:
		c.OnAdd(obj)
	case ControllerEventTypeUpdate:
		c.OnUpdate(obj)
	case ControllerEventTypeDelete:
		c.OnDelete(obj)
	default:
		log.Warn().Msgf("no event handler for '%s', event type '%s'", event.Key, event.Type)
	}
}

func (c *K8SController) runInformers() {
	c.factory.Start(nil)
	for _, ready := range c.factory.WaitForCacheSync(nil) {
		if !ready {
			runtime.HandleError(fmt.Errorf("[%s] Timed out waiting for caches to sync", c.id))
			return
		}
		log.Info().Msgf("[%s] Informer is ready and synced", c.id)
	}
}

// RunOnce starts the informer factory and syncs the data exactly once and then decrements the WaitGroup.
// It can also be cancelled early by the context.
func (c *K8SController) RunOnce(wg *sync.WaitGroup, ctx context.Context) {
	c.runInformers()

	go func() {
		defer runtime.HandleCrash()
		defer wg.Done()
		for {
			if c.queue.Len() == 0 {
				c.queue.SetExpired()
			}
			item, quit := c.queue.Pop(ctx)
			if quit {
				log.Debug().Msg("RunOnce: breaking")
				break
			}
			c.mainloop(item)
		}
	}()
}

// Run starts the informer factory and syncs data continuously until the context is cancelled.
func (c *K8SController) Run(ctx context.Context) {
	c.runInformers()

	go func() {
		defer runtime.HandleCrash()
		for {
			item, quit := c.queue.Pop(ctx)
			if quit {
				log.Debug().Msg("Run: breaking")
				break
			}
			c.mainloop(item)
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

	queue := queue.New()
	filter := NewK8SFilter(selector)
	factory := k8sClient.GetInformerFactory(resyncInterval)
	informer := factory.ForResource(*gvr).Informer()
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			queue.Insert(K8SEvent{
				Key:  key,
				Type: ControllerEventTypeCreate,
			})
		},
		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(old)
			if err != nil {
				return
			}
			queue.Insert(K8SEvent{
				Key:  key,
				Type: ControllerEventTypeUpdate,
			})
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			queue.Insert(K8SEvent{
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

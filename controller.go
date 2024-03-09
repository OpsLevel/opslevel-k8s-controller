package opslevel_k8s_controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type K8SControllerHandler func(interface{})

func nullKubernetesControllerHandler(item interface{}) {}

type K8SController struct {
	OnAdd    K8SControllerHandler
	OnDelete K8SControllerHandler
	OnUpdate K8SControllerHandler
	factory  dynamicinformer.DynamicSharedInformerFactory
	filter   *K8SFilter
	id       string
	informer cache.SharedIndexInformer
	log      *zerolog.Logger
	queue    *workqueue.Type
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
	log := c.log.With().Str("where", "mainloop").Int("queue_len", c.queue.Len()).Logger()
	log.Debug().Msg("mainloop: running from top")

	if _, ok := item.(K8SEvent); !ok {
		log.Warn().Msgf("cannot create K8SEvent from unknown interface '%T'", item)
		return
	}
	event := item.(K8SEvent)
	log = log.With().Str("k8s_event_key", event.Key).Str("k8s_event_type", string(event.Type)).Logger()
	indexer := c.informer.GetIndexer()
	obj, exists, err := indexer.GetByKey(event.Key)
	if err != nil {
		log.Error().Err(err).Msg("error fetching object from informer cache")
		return
	}
	if !exists {
		log.Debug().Msg("object skipped because it was not found")
		return
	}
	if c.filter.Matches(obj) {
		log.Debug().Msg("object skipped because it matches filter")
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
		log.Warn().Msg("no event handler for key and type")
	}
}

// Start starts the informer factory and sends events in the queue to the main loop.
// If a wait group is passed, Start will decrement it once it processes all events
// in the queue after one loop.
// If a wait group is not passed, Start will run continuously until the passed context
// is interrupted.
func (c *K8SController) Start(ctx context.Context, wg *sync.WaitGroup) {
	c.factory.Start(nil) // Starts all informers
	for _, ready := range c.factory.WaitForCacheSync(nil) {
		c.log.With().Str("where", "start.wait_for_cache_sync").Logger()
		if !ready {
			runtime.HandleError(fmt.Errorf("[%s] Timed out waiting for caches to sync", c.id))
			return
		}
		c.log.Info().Msg("Informer is ready and synced")
	}
	go func() {
		defer runtime.HandleCrash()
		if wg != nil {
			defer wg.Done()
		}
		var item interface{}
		var quit bool
	body:
		for {
			// This is a blocking operation performed in a goroutine to get the next event
			itemCh := make(chan interface{}, 1)
			quitCh := make(chan bool, 1)
			go func() {
				item, quit = c.queue.Get()
				itemCh <- item
				quitCh <- quit
			}()

			// Stop execution if the context is cancelled before get event finishes
			select {
			case <-ctx.Done():
				c.log.Debug().Msg("Breaking: on signal")
				break body
			case item = <-itemCh:
				quit = <-quitCh
			}
			if quit {
				c.log.Debug().Msg("Breaking: on quit")
				break
			}
			c.mainloop(item)
			c.queue.Done(item)
		}
	}()
	if wg != nil {
		c.queue.ShutDownWithDrain()
	}
}

// NewK8SController instantiates a new controller. Pass in a log or zerolog.Nop() to disable logging.
func NewK8SController(logger *zerolog.Logger, selector K8SSelector, resyncInterval time.Duration) (*K8SController, error) {
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
	controllerId := fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
	log := logger.With().Str("controller_id", controllerId).Logger()
	return &K8SController{
		OnAdd:    nullKubernetesControllerHandler,
		OnDelete: nullKubernetesControllerHandler,
		OnUpdate: nullKubernetesControllerHandler,
		factory:  factory,
		filter:   filter,
		id:       controllerId,
		informer: informer,
		log:      &log,
		queue:    queue,
	}, err
}

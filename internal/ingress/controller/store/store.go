/*
Copyright 2017 The Kubernetes Authors.
Copyright 2022-2023 The Alibaba Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/klog"

	"github.com/eapache/channels"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress-nginx/internal/astsutils"
	ingcheckv1 "k8s.io/ingress-nginx/internal/checksum/ingress/apis/checksum/v1"
	ingcheckclient "k8s.io/ingress-nginx/internal/checksum/ingress/client/clientset/versioned"
	ingcheckscheme "k8s.io/ingress-nginx/internal/checksum/ingress/client/clientset/versioned/scheme"
	ingcheckinformers "k8s.io/ingress-nginx/internal/checksum/ingress/client/informers/externalversions"
	secretcheckv1 "k8s.io/ingress-nginx/internal/checksum/secret/apis/checksum/v1"
	secretcheckclient "k8s.io/ingress-nginx/internal/checksum/secret/client/clientset/versioned"
	secretcheckscheme "k8s.io/ingress-nginx/internal/checksum/secret/client/clientset/versioned/scheme"
	secretcheckinformers "k8s.io/ingress-nginx/internal/checksum/secret/client/informers/externalversions"
	"k8s.io/ingress-nginx/internal/file"
	"k8s.io/ingress-nginx/internal/ingress"
	"k8s.io/ingress-nginx/internal/ingress/annotations"
	"k8s.io/ingress-nginx/internal/ingress/annotations/class"
	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	ngx_config "k8s.io/ingress-nginx/internal/ingress/controller/config"
	ngx_template "k8s.io/ingress-nginx/internal/ingress/controller/template"
	"k8s.io/ingress-nginx/internal/ingress/defaults"
	"k8s.io/ingress-nginx/internal/ingress/errors"
	"k8s.io/ingress-nginx/internal/ingress/metric"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
	"k8s.io/ingress-nginx/internal/ingress/secannotations"
	"k8s.io/ingress-nginx/internal/k8s"
	"k8s.io/ingress-nginx/internal/nginx"
)

// IngressFilterFunc decides if an Ingress should be omitted or not
type IngressFilterFunc func(*ingress.Ingress) bool

// IngressCheckFilterFunc decides if an IngressCheckSum should be omitted or not
type IngressCheckFilterFunc func(*ingcheckv1.IngressCheckSum) bool

// SecretCheckFilterFunc decides if an SecretCheckSum should be omitted or not
type SecretCheckFilterFunc func(*secretcheckv1.SecretCheckSum) bool

// Storer is the interface that wraps the required methods to gather information
// about ingresses, services, secrets and ingress annotations.
type Storer interface {
	// GetBackendConfiguration returns the nginx configuration stored in a configmap
	GetBackendConfiguration() ngx_config.Configuration

	// GetConfigMap returns the ConfigMap matching key.
	GetConfigMap(key string) (*corev1.ConfigMap, error)

	// GetSecret returns the Secret matching key.
	GetSecret(key string) (*corev1.Secret, error)

	// GetService returns the Service matching key.
	GetService(key string) (*corev1.Service, error)

	// GetServiceEndpoints returns the Endpoints of a Service matching key.
	GetServiceEndpoints(key string) (*corev1.Endpoints, error)

	// ListIngresses returns a list of all Ingresses in the store.
	ListIngresses(IngressFilterFunc) []*ingress.Ingress

	// GetIngressCheckSum returns the IngressCheckSum matching key.
	GetIngressCheckSum(key string) (*ingcheckv1.IngressCheckSum, error)

	// GetLocalIngressCheckSum returns the local cocy of a IngressCheckSum matching key.
	GetLocalIngressCheckSum(key string) (*ingcheckv1.IngressCheckSum, error)

	// ListLocalIngressCheckSums returns a list of local IngressCheckSums.
	ListLocalIngressCheckSums(IngressCheckFilterFunc) []*ingcheckv1.IngressCheckSum

	// ListIngsWithAnnotation returns a list of all Ingresses with annotations in the store.
	ListIngsWithAnnotation() []*ingress.Ingress

	// GetSecretWithAnnotation returns a secret with annotations in the store.
	GetSecretWithAnnotation(key string) (*ingress.Secret, error)

	// ListSecretsWithAnnotation returns a list of all Secrets with annotations in the store.
	ListSecretsWithAnnotation() []*ingress.Secret

	// GetSecretCheckSum returns the SecretCheckSum matching key.
	GetSecretCheckSum(key string) (*secretcheckv1.SecretCheckSum, error)

	// GetLocalSecretCheckSum returns the local cocy of a SecretCheckSum matching key.
	GetLocalSecretCheckSum(key string) (*secretcheckv1.SecretCheckSum, error)

	// ListLocalSecretCheckSums returns a list of local SecretCheckSums.
	ListLocalSecretCheckSums(SecretCheckFilterFunc) []*secretcheckv1.SecretCheckSum

	// GetRunningControllerPodsCount returns the number of Running ingress-nginx controller Pods.
	GetRunningControllerPodsCount() int

	// GetLocalSSLCert returns the local copy of a SSLCert
	GetLocalSSLCert(name string) (*ingress.SSLCert, error)

	// ListLocalSSLCerts returns the list of local SSLCerts
	ListLocalSSLCerts() []*ingress.SSLCert

	// GetAuthCertificate resolves a given secret name into an SSL certificate.
	// The secret must contain 3 keys named:
	//   ca.crt: contains the certificate chain used for authentication
	GetAuthCertificate(string) (*resolver.AuthSSLCert, error)

	// GetDefaultBackend returns the default backend configuration
	GetDefaultBackend() defaults.Backend

	// Run initiates the synchronization of the controllers
	Run(stopCh chan struct{})
}

// EventType type of event associated with an informer
type EventType string

const (
	// CreateEvent event associated with new objects in an informer
	CreateEvent EventType = "CREATE"
	// UpdateEvent event associated with an object update in an informer
	UpdateEvent EventType = "UPDATE"
	// DeleteEvent event associated when an object is removed from an informer
	DeleteEvent EventType = "DELETE"
	// ConfigurationEvent event associated when a controller configuration object is created or updated
	ConfigurationEvent EventType = "CONFIGURATION"
)

// Event holds the context of an event.
type Event struct {
	Type EventType
	Obj  interface{}
}

// Informer defines the required SharedIndexInformers that interact with the API server.
type Informer struct {
	Ingress         cache.SharedIndexInformer
	Endpoint        cache.SharedIndexInformer
	Service         cache.SharedIndexInformer
	Secret          cache.SharedIndexInformer
	ConfigMap       cache.SharedIndexInformer
	Pod             cache.SharedIndexInformer
	IngressCheckSum cache.SharedIndexInformer
	SecretCheckSum  cache.SharedIndexInformer
}

// Lister contains object listers (stores).
type Lister struct {
	Ingress               IngressLister
	Service               ServiceLister
	Endpoint              EndpointLister
	Secret                SecretLister
	ConfigMap             ConfigMapLister
	IngressWithAnnotation IngressWithAnnotationsLister
	Pod                   PodLister
	IngressCheckSum       IngressCheckSumLister
	SecretCheckSum        SecretCheckSumLister
	IngWithAnnotation     IngressWithAnnotationsLister
	SecretWithAnnotation  SecretWithAnnotationsLister
}

// NotExistsError is returned when an object does not exist in a local store.
type NotExistsError string

// Error implements the error interface.
func (e NotExistsError) Error() string {
	return fmt.Sprintf("no object matching key %q in local store", string(e))
}

var useIngCheckSum = false
var useSecretCheckSum = false

// Run initiates the synchronization of the informers against the API server.
func (i *Informer) Run(stopCh chan struct{}) {
	go i.Secret.Run(stopCh)
	go i.Endpoint.Run(stopCh)
	go i.Service.Run(stopCh)
	go i.ConfigMap.Run(stopCh)
	go i.Pod.Run(stopCh)

	// wait for all involved caches to be synced before processing items
	// from the queue
	if !cache.WaitForCacheSync(stopCh,
		i.Endpoint.HasSynced,
		i.Service.HasSynced,
		i.Secret.HasSynced,
		i.ConfigMap.HasSynced,
		i.Pod.HasSynced,
	) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
	}

	if useIngCheckSum {
		go i.IngressCheckSum.Run(stopCh)
	}

	if useSecretCheckSum {
		go i.SecretCheckSum.Run(stopCh)
	}

	if useIngCheckSum && !cache.WaitForCacheSync(stopCh,
		i.IngressCheckSum.HasSynced,
	) {
		klog.Errorf("CRD IngressCheckSum is not ready")
	}

	if useSecretCheckSum && !cache.WaitForCacheSync(stopCh,
		i.SecretCheckSum.HasSynced,
	) {
		klog.Errorf("CRD SecretCheckSum is not ready")
	}

	// in big clusters, deltas can keep arriving even after HasSynced
	// functions have returned 'true'
	time.Sleep(1 * time.Second)

	// we can start syncing ingress objects only after other caches are
	// ready, because ingress rules require content from other listers, and
	// 'add' events get triggered in the handlers during caches population.
	go i.Ingress.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh,
		i.Ingress.HasSynced,
	) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
	}
}

// k8sStore internal Storer implementation using informers and thread safe stores
type k8sStore struct {
	// backendConfig contains the running configuration from the configmap
	// this is required because this rarely changes but is a very expensive
	// operation to execute in each OnUpdate invocation
	backendConfig ngx_config.Configuration

	// informer contains the cache Informers
	informers *Informer

	// listers contains the cache.Store interfaces used in the ingress controller
	listers *Lister

	// sslStore local store of SSL certificates (certificates used in ingress)
	// this is required because the certificates must be present in the
	// container filesystem
	sslStore *SSLCertTracker

	annotations annotations.Extractor

	// secretIngressMap contains information about which ingress references a
	// secret in the annotations.
	secretIngressMap ObjectRefMap

	// updateCh
	updateCh *channels.RingChannel

	// syncSecretMu protects against simultaneous invocations of syncSecret
	syncSecretMu *sync.Mutex

	// backendConfigMu protects against simultaneous read/write of backendConfig
	backendConfigMu *sync.RWMutex

	defaultSSLCertificate string

	pod *k8s.PodInfo

	// ingCheckSumStore local store of ingress checkesum
	ingCheckSumStore *IngressCheckSumStore

	// secretCheckSumStore local store of secret checkesum
	secretCheckSumStore *SecretCheckSumStore

	// secAnnotations defines the annotation parsers for secret
	secAnnotations secannotations.Extractor

	mc metric.Collector

	checksumStatus *ingress.ChecksumStatus
}

// New creates a new object store to be used in the ingress controller
func New(
	namespace, configmap, tcp, udp, defaultSSLCertificate string,
	resyncPeriod time.Duration,
	client clientset.Interface,
	ClientIng clientset.Interface,
	ClientIngCheck ingcheckclient.Interface,
	ClientSecretCheck secretcheckclient.Interface,
	mc metric.Collector,
	updateCh *channels.RingChannel,
	pod *k8s.PodInfo,
	disableCatchAll bool,
	checksumStatus *ingress.ChecksumStatus) Storer {

	store := &k8sStore{
		informers:             &Informer{},
		listers:               &Lister{},
		sslStore:              NewSSLCertTracker(),
		updateCh:              updateCh,
		backendConfig:         ngx_config.NewDefault(),
		syncSecretMu:          &sync.Mutex{},
		backendConfigMu:       &sync.RWMutex{},
		secretIngressMap:      NewObjectRefMap(),
		defaultSSLCertificate: defaultSSLCertificate,
		pod:                   pod,
		ingCheckSumStore:      NewIngressCheckSumStore(),
		secretCheckSumStore:   NewSecretCheckSumStore(),
		mc:                    mc,
		checksumStatus:        checksumStatus,
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{
		Interface: client.CoreV1().Events(namespace),
	})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{
		Component: "tengine-ingress-controller",
	})

	ns, name, _ := k8s.ParseNameNS(configmap)
	cm, err := client.CoreV1().ConfigMaps(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Warningf("Unexpected error reading configuration configmap: %v", err)
	}

	store.setConfig(cm)

	// k8sStore fulfills resolver.Resolver interface
	store.annotations = annotations.NewAnnotationExtractor(store)

	store.listers.IngressWithAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	// create informers factory, enable and assign required informers
	infFactory := informers.NewSharedInformerFactoryWithOptions(client, resyncPeriod,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(*metav1.ListOptions) {}))

	store.listers.IngWithAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	store.secAnnotations = secannotations.NewAnnotationExtractor(store)
	store.listers.SecretWithAnnotation.Store = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ingFactory := informers.NewSharedInformerFactoryWithOptions(ClientIng, resyncPeriod,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(*metav1.ListOptions) {}))
	ingCheckCrdFactory := ingcheckinformers.NewSharedInformerFactoryWithOptions(ClientIngCheck, resyncPeriod,
		ingcheckinformers.WithNamespace(namespace),
		ingcheckinformers.WithTweakListOptions(func(*metav1.ListOptions) {}))
	secretCheckCrdFactory := secretcheckinformers.NewSharedInformerFactoryWithOptions(ClientSecretCheck, resyncPeriod,
		secretcheckinformers.WithNamespace(namespace),
		secretcheckinformers.WithTweakListOptions(func(*metav1.ListOptions) {}))

	useStorageCluster := store.GetBackendConfiguration().UseIngStorageCluster
	store.informers.Ingress = store.getIngInformer(useStorageCluster, infFactory, ingFactory)
	store.listers.Ingress.Store = store.informers.Ingress.GetStore()

	store.informers.IngressCheckSum = ingCheckCrdFactory.Tengine().V1().IngressCheckSums().Informer()
	store.listers.IngressCheckSum.Store = store.informers.IngressCheckSum.GetStore()

	store.informers.SecretCheckSum = secretCheckCrdFactory.Tengine().V1().SecretCheckSums().Informer()
	store.listers.SecretCheckSum.Store = store.informers.SecretCheckSum.GetStore()

	store.informers.Endpoint = infFactory.Core().V1().Endpoints().Informer()
	store.listers.Endpoint.Store = store.informers.Endpoint.GetStore()

	if useStorageCluster {
		store.informers.Secret = ingFactory.Core().V1().Secrets().Informer()
	} else {
		store.informers.Secret = infFactory.Core().V1().Secrets().Informer()
	}
	store.listers.Secret.Store = store.informers.Secret.GetStore()

	store.informers.ConfigMap = infFactory.Core().V1().ConfigMaps().Informer()
	store.listers.ConfigMap.Store = store.informers.ConfigMap.GetStore()

	store.informers.Service = infFactory.Core().V1().Services().Informer()
	store.listers.Service.Store = store.informers.Service.GetStore()

	labelSelector := labels.SelectorFromSet(store.pod.Labels)
	store.informers.Pod = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (k8sruntime.Object, error) {
				options.LabelSelector = labelSelector.String()
				return client.CoreV1().Pods(store.pod.Namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.LabelSelector = labelSelector.String()
				return client.CoreV1().Pods(store.pod.Namespace).Watch(context.TODO(), options)
			},
		},
		&corev1.Pod{},
		resyncPeriod,
		cache.Indexers{},
	)
	store.listers.Pod.Store = store.informers.Pod.GetStore()

	ingDeleteHandler := func(obj interface{}) {
		ing, ok := toIngress(obj)
		if !ok {
			// If we reached here it means the ingress was deleted but its final state is unrecorded.
			tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
			if !ok {
				klog.Errorf("couldn't get object from tombstone %#v", obj)
				return
			}
			ing, ok = tombstone.Obj.(*networkingv1.Ingress)
			if !ok {
				klog.Errorf("Tombstone contained object that is not an Ingress: %#v", obj)
				return
			}
		}

		if !class.IsValid(ing) {
			klog.Infof("ignoring delete for ingress %v based on annotation %v", ing.Name, class.IngressKey)
			return
		}
		if isCatchAllIngress(ing.Spec) && disableCatchAll {
			klog.Infof("ignoring delete for catch-all ingress %v/%v because of --disable-catch-all", ing.Namespace, ing.Name)
			return
		}
		recorder.Eventf(ing, corev1.EventTypeNormal, "DELETE", fmt.Sprintf("Ingress %s/%s", ing.Namespace, ing.Name))

		store.listers.IngWithAnnotation.Delete(ing)
		store.listers.IngressWithAnnotation.Delete(ing)

		key := k8s.MetaNamespaceKey(ing)
		store.secretIngressMap.Delete(key)

		updateCh.In() <- Event{
			Type: DeleteEvent,
			Obj:  obj,
		}
	}

	ingEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ing, _ := toIngress(obj)
			if !class.IsValid(ing) {
				a, _ := parser.GetStringAnnotation(class.IngressKey, ing)
				klog.Infof("ignoring add for ingress %v based on annotation %v with value %v", ing.Name, class.IngressKey, a)
				return
			}
			if isCatchAllIngress(ing.Spec) && disableCatchAll {
				klog.Infof("ignoring add for catch-all ingress %v/%v because of --disable-catch-all", ing.Namespace, ing.Name)
				return
			}
			recorder.Eventf(ing, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("Ingress %s/%s", ing.Namespace, ing.Name))

			store.updateIngWithAnnotation(ing)
			store.syncIngress(ing)
			store.updateSecretIngressMap(ing)
			store.syncSecrets(ing)

			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: ingDeleteHandler,
		UpdateFunc: func(old, cur interface{}) {
			oldIng, _ := toIngress(old)
			curIng, _ := toIngress(cur)

			validOld := class.IsValid(oldIng)
			validCur := class.IsValid(curIng)
			if !validOld && validCur {
				if isCatchAllIngress(curIng.Spec) && disableCatchAll {
					klog.Infof("ignoring update for catch-all ingress %v/%v because of --disable-catch-all", curIng.Namespace, curIng.Name)
					return
				}

				klog.Infof("creating ingress %v based on annotation %v", curIng.Name, class.IngressKey)
				recorder.Eventf(curIng, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("Ingress %s/%s", curIng.Namespace, curIng.Name))
			} else if validOld && !validCur {
				klog.Infof("removing ingress %v based on annotation %v", curIng.Name, class.IngressKey)
				ingDeleteHandler(old)
				return
			} else if validCur && !reflect.DeepEqual(old, cur) {
				if isCatchAllIngress(curIng.Spec) && disableCatchAll {
					klog.Infof("ignoring update for catch-all ingress %v/%v and delete old one because of --disable-catch-all", curIng.Namespace, curIng.Name)
					ingDeleteHandler(old)
					return
				}

				recorder.Eventf(curIng, corev1.EventTypeNormal, "UPDATE", fmt.Sprintf("Ingress %s/%s", curIng.Namespace, curIng.Name))
			} else {
				klog.V(3).Infof("No changes on ingress %v/%v. Skipping update", curIng.Namespace, curIng.Name)
				return
			}

			store.updateIngWithAnnotation(curIng)
			store.syncIngress(curIng)
			store.updateSecretIngressMap(curIng)
			store.syncSecrets(curIng)

			updateCh.In() <- Event{
				Type: UpdateEvent,
				Obj:  cur,
			}
		},
	}

	secrEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			sec := obj.(*corev1.Secret)
			key := k8s.MetaNamespaceKey(sec)

			if store.defaultSSLCertificate == key {
				store.syncSecret(store.defaultSSLCertificate, mc)
			}

			store.updateSecretWithAnnotation(sec)

			// find references in ingresses and update local ssl certs
			if ings := store.secretIngressMap.Reference(key); len(ings) > 0 {
				klog.Infof("secret %v was added and it is used in ingress annotations. Parsing...", key)
				for _, ingKey := range ings {
					ing, err := store.getIngress(ingKey)
					if err != nil {
						klog.Errorf("could not find Ingress %v in local store", ingKey)
						continue
					}
					store.syncIngress(ing)
					store.syncSecrets(ing)
				}

				updateCh.In() <- Event{
					Type: CreateEvent,
					Obj:  obj,
				}
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				sec := cur.(*corev1.Secret)
				key := k8s.MetaNamespaceKey(sec)

				if store.defaultSSLCertificate == key {
					store.syncSecret(store.defaultSSLCertificate, mc)
				}

				store.updateSecretWithAnnotation(sec)

				// find references in ingresses and update local ssl certs
				if ings := store.secretIngressMap.Reference(key); len(ings) > 0 {
					klog.Infof("secret %v was updated and it is used in ingress annotations. Parsing...", key)
					for _, ingKey := range ings {
						ing, err := store.getIngress(ingKey)
						if err != nil {
							klog.Errorf("could not find Ingress %v in local store", ingKey)
							continue
						}
						store.syncIngress(ing)
						store.syncSecrets(ing)
					}

					updateCh.In() <- Event{
						Type: UpdateEvent,
						Obj:  cur,
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			sec, ok := obj.(*corev1.Secret)
			if !ok {
				// If we reached here it means the secret was deleted but its final state is unrecorded.
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					klog.Errorf("couldn't get object from tombstone %#v", obj)
					return
				}
				sec, ok = tombstone.Obj.(*corev1.Secret)
				if !ok {
					klog.Errorf("Tombstone contained object that is not a Secret: %#v", obj)
					return
				}
			}

			store.listers.SecretWithAnnotation.Delete(sec)
			store.sslStore.Delete(k8s.MetaNamespaceKey(sec))

			key := k8s.MetaNamespaceKey(sec)
			// find references in ingresses
			if ings := store.secretIngressMap.Reference(key); len(ings) > 0 {
				klog.Infof("secret %v was deleted and it is used in ingress annotations. Parsing...", key)
				for _, ingKey := range ings {
					ing, err := store.getIngress(ingKey)
					if err != nil {
						klog.Errorf("could not find Ingress %v in local store", ingKey)
						continue
					}
					store.syncIngress(ing)
				}

				updateCh.In() <- Event{
					Type: DeleteEvent,
					Obj:  obj,
				}
			}
		},
	}

	icEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ingCheckSum, ok := obj.(*ingcheckv1.IngressCheckSum)
			if !ok {
				klog.Errorf("couldn't get object from ingCheckSum %#v for CREATE operation", obj)
				return
			}

			key := k8s.MetaNamespaceKey(ingCheckSum)
			recorder.Eventf(ingCheckSum, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("IngressCheckSum %v", key))
			store.ingCheckSumStore.Add(key, ingCheckSum)
			if !store.checksumStatus.IngChecksumStatus {
				updateCh.In() <- Event{
					Type: CreateEvent,
					Obj:  obj,
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			ingCheckSum, ok := obj.(*ingcheckv1.IngressCheckSum)
			if !ok {
				klog.Errorf("couldn't get object from ingCheckSum %#v for DELETE operation", obj)
				return
			}

			key := k8s.MetaNamespaceKey(ingCheckSum)
			recorder.Eventf(ingCheckSum, corev1.EventTypeNormal, "DELETE", fmt.Sprintf("IngressCheckSum %v", key))
			store.ingCheckSumStore.Delete(key)
		},
	}

	scEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secretCheckSum, ok := obj.(*secretcheckv1.SecretCheckSum)
			if !ok {
				klog.Errorf("couldn't get object from secretCheckSum %#v for CREATE operation", obj)
				return
			}

			key := k8s.MetaNamespaceKey(secretCheckSum)
			recorder.Eventf(secretCheckSum, corev1.EventTypeNormal, "CREATE", fmt.Sprintf("SecretCheckSum %v", key))
			store.secretCheckSumStore.Add(key, secretCheckSum)
			if !store.checksumStatus.SecretChecksumStatus {
				updateCh.In() <- Event{
					Type: CreateEvent,
					Obj:  obj,
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			secretCheckSum, ok := obj.(*secretcheckv1.SecretCheckSum)
			if !ok {
				klog.Errorf("couldn't get object from secretCheckSum %#v for DELETE operation", obj)
				return
			}

			key := k8s.MetaNamespaceKey(secretCheckSum)
			recorder.Eventf(secretCheckSum, corev1.EventTypeNormal, "DELETE", fmt.Sprintf("SecretCheckSum %v", key))
			store.secretCheckSumStore.Delete(key)
		},
	}

	epEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			oep := old.(*corev1.Endpoints)
			cep := cur.(*corev1.Endpoints)
			if !reflect.DeepEqual(cep.Subsets, oep.Subsets) {
				updateCh.In() <- Event{
					Type: UpdateEvent,
					Obj:  cur,
				}
			}
		},
	}

	// TODO: add e2e test to verify that changes to one or more configmap trigger an update
	changeTriggerUpdate := func(name string) bool {
		if name == configmap {
		}
		return name == configmap || name == tcp || name == udp
	}

	handleCfgMapEvent := func(key string, cfgMap *corev1.ConfigMap, eventName string) {
		// updates to configuration configmaps can trigger an update
		triggerUpdate := false
		if changeTriggerUpdate(key) {
			triggerUpdate = true
			recorder.Eventf(cfgMap, corev1.EventTypeNormal, eventName, fmt.Sprintf("ConfigMap %v", key))
			if key == configmap {
				store.setConfig(cfgMap)
			}
		}

		ings := store.listers.IngressWithAnnotation.List()
		for _, ingKey := range ings {
			key := k8s.MetaNamespaceKey(ingKey)
			ing, err := store.getIngress(key)
			if err != nil {
				klog.Errorf("could not find Ingress %v in local store: %v", key, err)
				continue
			}

			if parser.AnnotationsReferencesConfigmap(ing) {
				store.syncIngress(ing)
				continue
			}

			if triggerUpdate {
				store.syncIngress(ing)
			}
		}

		if triggerUpdate {
			updateCh.In() <- Event{
				Type: ConfigurationEvent,
				Obj:  cfgMap,
			}
		}
	}

	cmEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cfgMap := obj.(*corev1.ConfigMap)
			key := k8s.MetaNamespaceKey(cfgMap)
			handleCfgMapEvent(key, cfgMap, "CREATE")
		},
		UpdateFunc: func(old, cur interface{}) {
			if reflect.DeepEqual(old, cur) {
				return
			}

			cfgMap := cur.(*corev1.ConfigMap)
			key := k8s.MetaNamespaceKey(cfgMap)
			handleCfgMapEvent(key, cfgMap, "UPDATE")
		},
	}

	podEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			oldPod := old.(*corev1.Pod)
			curPod := cur.(*corev1.Pod)

			if oldPod.Status.Phase == curPod.Status.Phase {
				return
			}

			updateCh.In() <- Event{
				Type: UpdateEvent,
				Obj:  cur,
			}
		},
		DeleteFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
	}

	store.informers.Ingress.AddEventHandler(ingEventHandler)
	store.informers.IngressCheckSum.AddEventHandler(icEventHandler)
	store.informers.SecretCheckSum.AddEventHandler(scEventHandler)
	store.informers.Endpoint.AddEventHandler(epEventHandler)
	store.informers.Secret.AddEventHandler(secrEventHandler)
	store.informers.ConfigMap.AddEventHandler(cmEventHandler)
	store.informers.Service.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	store.informers.Pod.AddEventHandler(podEventHandler)

	useIngCheckSum = store.GetBackendConfiguration().UseIngCheckSum
	useSecretCheckSum = store.GetBackendConfiguration().UseSecretCheckSum
	return store
}

// isCatchAllIngress returns whether or not an ingress produces a
// catch-all server, and so should be ignored when --disable-catch-all is set
func isCatchAllIngress(spec networkingv1.IngressSpec) bool {
	return spec.DefaultBackend != nil && len(spec.Rules) == 0
}

// syncIngress parses ingress annotations converting the value of the
// annotation to a go struct
func (s *k8sStore) syncIngress(ing *networkingv1.Ingress) {
	key := k8s.MetaNamespaceKey(ing)
	klog.V(3).Infof("updating annotations information for ingress %v", key)

	anns := s.annotations.Extract(ing)
	if !s.verifyIngressReferrer(key, anns) {
		return
	}

	gray, _ := s.GetIngressGrayStatus(key, anns)
	if gray.Type == ingress.InactiveGray {
		klog.Infof("Ingress %v is marked as inactive gray, ignoring", key)
		return
	}

	copyIng := &networkingv1.Ingress{}
	ing.ObjectMeta.DeepCopyInto(&copyIng.ObjectMeta)
	ing.Spec.DeepCopyInto(&copyIng.Spec)
	ing.Status.DeepCopyInto(&copyIng.Status)

	for ri, rule := range copyIng.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		for pi, path := range rule.HTTP.Paths {
			if path.Path == "" {
				copyIng.Spec.Rules[ri].HTTP.Paths[pi].Path = "/"
			}
		}
	}

	err := s.listers.IngressWithAnnotation.Update(&ingress.Ingress{
		Ingress:           *copyIng,
		ParsedAnnotations: s.annotations.Extract(ing),
	})
	if err != nil {
		klog.Error(err)
	}
}

// updateSecretIngressMap takes an Ingress and updates all Secret objects it
// references in secretIngressMap.
func (s *k8sStore) updateSecretIngressMap(ing *networkingv1.Ingress) {
	key := k8s.MetaNamespaceKey(ing)
	klog.V(3).Infof("updating references to secrets for ingress %v", key)

	// delete all existing references first
	s.secretIngressMap.Delete(key)

	var refSecrets []string

	for _, tls := range ing.Spec.TLS {
		secrName := tls.SecretName
		if secrName != "" {
			secrKey := fmt.Sprintf("%v/%v", ing.Namespace, secrName)
			refSecrets = append(refSecrets, secrKey)
		}
	}

	// We can not rely on cached ingress annotations because these are
	// discarded when the referenced secret does not exist in the local
	// store. As a result, adding a secret *after* the ingress(es) which
	// references it would not trigger a resync of that secret.
	secretAnnotations := []string{
		"auth-secret",
		"auth-tls-secret",
		"proxy-ssl-secret",
		"secure-verify-ca-secret",
	}
	for _, ann := range secretAnnotations {
		secrKey, err := objectRefAnnotationNsKey(ann, ing)
		if err != nil && !errors.IsMissingAnnotations(err) {
			klog.Errorf("error reading secret reference in annotation %q: %s", ann, err)
			continue
		}
		if secrKey != "" {
			refSecrets = append(refSecrets, secrKey)
		}
	}

	// populate map with all secret references
	s.secretIngressMap.Insert(key, refSecrets...)
}

// objectRefAnnotationNsKey returns an object reference formatted as a
// 'namespace/name' key from the given annotation name.
func objectRefAnnotationNsKey(ann string, ing *networkingv1.Ingress) (string, error) {
	annValue, err := parser.GetStringAnnotation(ann, ing)
	if err != nil {
		return "", err
	}

	secrNs, secrName, err := cache.SplitMetaNamespaceKey(annValue)
	if secrName == "" {
		return "", err
	}

	if secrNs == "" {
		return fmt.Sprintf("%v/%v", ing.Namespace, secrName), nil
	}
	return annValue, nil
}

// syncSecrets synchronizes data from all Secrets referenced by the given
// Ingress with the local store and file system.
func (s *k8sStore) syncSecrets(ing *networkingv1.Ingress) {
	key := k8s.MetaNamespaceKey(ing)
	for _, secrKey := range s.secretIngressMap.ReferencedBy(key) {
		gray, _ := s.GetSecretGrayStatus(secrKey)
		if gray.Type == ingress.InactiveGray {
			klog.Infof("Secret %v is marked as inactive gray, ignoring", secrKey)
			continue
		}

		s.syncSecret(secrKey, s.mc)
	}
}

// GetSecret returns the Secret matching key.
func (s *k8sStore) GetSecret(key string) (*corev1.Secret, error) {
	return s.listers.Secret.ByKey(key)
}

// ListLocalSSLCerts returns the list of local SSLCerts
func (s *k8sStore) ListLocalSSLCerts() []*ingress.SSLCert {
	var certs []*ingress.SSLCert
	for _, item := range s.sslStore.List() {
		if s, ok := item.(*ingress.SSLCert); ok {
			certs = append(certs, s)
		}
	}

	return certs
}

// GetService returns the Service matching key.
func (s *k8sStore) GetService(key string) (*corev1.Service, error) {
	return s.listers.Service.ByKey(key)
}

// getIngress returns the Ingress matching key.
func (s *k8sStore) getIngress(key string) (*networkingv1.Ingress, error) {
	ing, err := s.listers.IngressWithAnnotation.ByKey(key)
	if err != nil {
		return nil, err
	}

	return &ing.Ingress, nil
}

// ListIngresses returns the list of Ingresses
func (s *k8sStore) ListIngresses(filter IngressFilterFunc) []*ingress.Ingress {
	// filter ingress rules
	ingresses := make([]*ingress.Ingress, 0)
	for _, item := range s.listers.IngressWithAnnotation.List() {
		ing := item.(*ingress.Ingress)

		if filter != nil && filter(ing) {
			continue
		}

		ingresses = append(ingresses, ing)
	}

	// sort Ingresses using the CreationTimestamp field
	sort.SliceStable(ingresses, func(i, j int) bool {
		ir := ingresses[i].CreationTimestamp
		jr := ingresses[j].CreationTimestamp
		if ir.Equal(&jr) {
			in := fmt.Sprintf("%v/%v", ingresses[i].Namespace, ingresses[i].Name)
			jn := fmt.Sprintf("%v/%v", ingresses[j].Namespace, ingresses[j].Name)
			klog.V(3).Infof("Ingress %v and %v have identical CreationTimestamp", in, jn)
			return in > jn
		}
		return ir.Before(&jr)
	})

	return ingresses
}

// ListLocalIngressCheckSums returns a list of local IngressCheckSums.
func (s *k8sStore) ListLocalIngressCheckSums(filter IngressCheckFilterFunc) []*ingcheckv1.IngressCheckSum {
	// filter IngressCheckSum
	var ingressCheckSums []*ingcheckv1.IngressCheckSum
	for _, item := range s.ingCheckSumStore.List() {
		ingCheckSum := item.(*ingcheckv1.IngressCheckSum)
		if filter != nil && filter(ingCheckSum) {
			continue
		}

		ingressCheckSums = append(ingressCheckSums, ingCheckSum)
	}

	// reverse-order IngressCheckSum using the CreationTimestamp field
	sort.SliceStable(ingressCheckSums, func(i, j int) bool {
		ir := ingressCheckSums[i].CreationTimestamp
		jr := ingressCheckSums[j].CreationTimestamp
		if ir.Equal(&jr) {
			in := fmt.Sprintf("%v/%v", ingressCheckSums[i].Namespace, ingressCheckSums[i].Name)
			jn := fmt.Sprintf("%v/%v", ingressCheckSums[j].Namespace, ingressCheckSums[j].Name)
			klog.V(3).Infof("IngressCheckSum %v and %v have identical CreationTimestamp", in, jn)
			return in < jn
		}
		return !ir.Before(&jr)
	})

	return ingressCheckSums
}

// GetIngressCheckSum returns the IngressCheckSum matching key.
func (s *k8sStore) GetIngressCheckSum(key string) (*ingcheckv1.IngressCheckSum, error) {
	return s.listers.IngressCheckSum.ByKey(key)
}

// GetLocalIngressCheckSum returns the local cocy of a IngressCheckSum matching key.
func (s *k8sStore) GetLocalIngressCheckSum(key string) (*ingcheckv1.IngressCheckSum, error) {
	return s.ingCheckSumStore.ByKey(key)
}

// GetLocalSSLCert returns the local copy of a SSLCert
func (s *k8sStore) GetLocalSSLCert(key string) (*ingress.SSLCert, error) {
	return s.sslStore.ByKey(key)
}

// GetConfigMap returns the ConfigMap matching key.
func (s *k8sStore) GetConfigMap(key string) (*corev1.ConfigMap, error) {
	return s.listers.ConfigMap.ByKey(key)
}

// GetServiceEndpoints returns the Endpoints of a Service matching key.
func (s *k8sStore) GetServiceEndpoints(key string) (*corev1.Endpoints, error) {
	return s.listers.Endpoint.ByKey(key)
}

// GetAuthCertificate is used by the auth-tls annotations to get a cert from a secret
func (s *k8sStore) GetAuthCertificate(name string) (*resolver.AuthSSLCert, error) {
	if _, err := s.GetLocalSSLCert(name); err != nil {
		s.syncSecret(name, s.mc)
	}

	cert, err := s.GetLocalSSLCert(name)
	if err != nil {
		return nil, err
	}

	return &resolver.AuthSSLCert{
		Secret:      name,
		CAFileName:  cert.CAFileName,
		CASHA:       cert.CASHA,
		CRLFileName: cert.CRLFileName,
		CRLSHA:      cert.CRLSHA,
		PemFileName: cert.PemFileName,
	}, nil
}

func (s *k8sStore) writeSSLSessionTicketKey(cmap *corev1.ConfigMap, fileName string) {
	ticketString := ngx_template.ReadConfig(cmap.Data).SSLSessionTicketKey
	s.backendConfig.SSLSessionTicketKey = ""

	if ticketString != "" {
		ticketBytes := base64.StdEncoding.WithPadding(base64.StdPadding).DecodedLen(len(ticketString))

		// 81 used instead of 80 because of padding
		if !(ticketBytes == 48 || ticketBytes == 81) {
			klog.Warningf("ssl-session-ticket-key must contain either 48 or 80 bytes")
		}

		decodedTicket, err := base64.StdEncoding.DecodeString(ticketString)
		if err != nil {
			klog.Errorf("unexpected error decoding ssl-session-ticket-key: %v", err)
			return
		}

		err = ioutil.WriteFile(fileName, decodedTicket, file.ReadWriteByUser)
		if err != nil {
			klog.Errorf("unexpected error writing ssl-session-ticket-key to %s: %v", fileName, err)
			return
		}

		s.backendConfig.SSLSessionTicketKey = ticketString
	}
}

// GetDefaultBackend returns the default backend
func (s *k8sStore) GetDefaultBackend() defaults.Backend {
	return s.GetBackendConfiguration().Backend
}

func (s *k8sStore) GetBackendConfiguration() ngx_config.Configuration {
	s.backendConfigMu.RLock()
	defer s.backendConfigMu.RUnlock()

	return s.backendConfig
}

func (s *k8sStore) setConfig(cmap *corev1.ConfigMap) {
	s.backendConfigMu.Lock()
	defer s.backendConfigMu.Unlock()

	if cmap == nil {
		return
	}

	s.backendConfig = ngx_template.ReadConfig(cmap.Data)
	if s.backendConfig.UseGeoIP2 && !nginx.GeoLite2DBExists() {
		klog.Warning("The GeoIP2 feature is enabled but the databases are missing. Disabling.")
		s.backendConfig.UseGeoIP2 = false
	}

	s.writeSSLSessionTicketKey(cmap, "/etc/nginx/tickets.key")
}

// Run initiates the synchronization of the informers and the initial
// synchronization of the secrets.
func (s *k8sStore) Run(stopCh chan struct{}) {
	// start informers
	s.informers.Run(stopCh)
}

// GetRunningControllerPodsCount returns the number of Running ingress-nginx controller Pods
func (s k8sStore) GetRunningControllerPodsCount() int {
	count := 0

	for _, i := range s.listers.Pod.List() {
		pod := i.(*corev1.Pod)

		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		count++
	}

	return count
}

var runtimeScheme = k8sruntime.NewScheme()

func init() {
	networkingv1.AddToScheme(runtimeScheme)
	ingcheckv1.AddToScheme(runtimeScheme)
	runtime.Must(ingcheckscheme.AddToScheme(scheme.Scheme))
	secretcheckv1.AddToScheme(runtimeScheme)
	runtime.Must(secretcheckscheme.AddToScheme(scheme.Scheme))
}

func toIngress(obj interface{}) (*networkingv1.Ingress, bool) {
	if ing, ok := obj.(*networkingv1.Ingress); ok {
		return ing, true
	}

	return nil, false
}

// updateSecretWithAnnotation updates Secret with annotations
func (s *k8sStore) updateSecretWithAnnotation(secret *corev1.Secret) {
	key := k8s.MetaNamespaceKey(secret)
	klog.Infof("updating annotations information for secret [%v]", key)

	cert, err := s.getPemCertificate(key)
	if err != nil {
		klog.Errorf("failed to obtain X.509 certificate [%v]: %v", key, err)
		return
	}

	err = s.listers.SecretWithAnnotation.Update(&ingress.Secret{
		Secret:            *secret,
		SSLCert:           cert,
		ParsedAnnotations: s.secAnnotations.Extract(secret),
	})
	if err != nil {
		klog.Error("update secret with annotations failed: ", err)
	}
}

// GetSecretWithAnnotation returns a secret with annotations in the store
func (s *k8sStore) GetSecretWithAnnotation(key string) (*ingress.Secret, error) {
	secret, err := s.listers.SecretWithAnnotation.ByKey(key)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// updateIngWithAnnotation updates Ingress with annotations
func (s *k8sStore) updateIngWithAnnotation(ing *networkingv1.Ingress) {
	key := k8s.MetaNamespaceKey(ing)
	klog.Infof("updating annotations information for ingress [%v]", key)

	anns := s.annotations.Extract(ing)
	if !s.verifyIngressReferrer(key, anns) {
		return
	}

	err := s.listers.IngWithAnnotation.Update(&ingress.Ingress{
		Ingress:           *ing,
		ParsedAnnotations: s.annotations.Extract(ing),
	})
	if err != nil {
		klog.Error("update ingress with annotations failed: ", err)
	}
}

// ListIngsWithAnnotation returns the list of Ingresses with annotations
func (s *k8sStore) ListIngsWithAnnotation() []*ingress.Ingress {
	ingresses := make([]*ingress.Ingress, 0)
	for _, item := range s.listers.IngWithAnnotation.List() {
		ing := item.(*ingress.Ingress)
		ingresses = append(ingresses, ing)
	}

	// sort Ingresses using the CreationTimestamp field
	sort.SliceStable(ingresses, func(i, j int) bool {
		ir := ingresses[i].CreationTimestamp
		jr := ingresses[j].CreationTimestamp
		if ir.Equal(&jr) {
			in := fmt.Sprintf("%v/%v", ingresses[i].Namespace, ingresses[i].Name)
			jn := fmt.Sprintf("%v/%v", ingresses[j].Namespace, ingresses[j].Name)
			klog.V(3).Infof("Ingress %v and %v have identical CreationTimestamp", in, jn)
			return in > jn
		}
		return ir.Before(&jr)
	})

	return ingresses
}

// verifyIngressReferrer used to verify the ingress referrer
func (s *k8sStore) verifyIngressReferrer(key string, anns *annotations.Ingress) bool {
	if anns.Referrer.IngReferrer == "" {
		klog.Infof("Ingress[%v] with empty referrer", key)
		return true
	}

	ingReferrers := strings.Split(s.GetBackendConfiguration().IngressReferrer, ",")
	for _, ingReferrer := range ingReferrers {
		if ingReferrer == anns.Referrer.IngReferrer {
			return true
		}
	}

	s.mc.IncIngReferInvalidCount()
	klog.Warningf("Ingress[%v] with referrer[%v] is illegal, ignored", key, anns.Referrer.IngReferrer)
	return false
}

// GetIngressGrayStatus returns gray status of a ingress
func (s *k8sStore) GetIngressGrayStatus(key string, anns *annotations.Ingress) (ingress.IngGray, error) {
	gray := ingress.IngGray{
		Type: ingress.Active,
	}

	podOrdinal := astsutils.GetPodOrdinal(s.pod)
	ingGrayIndex := int32(anns.IngGray.IngGrayIndex)
	if !anns.IngGray.IngGrayFlag {
		gray.Type = ingress.Active
	} else if ingGrayIndex > 0 && podOrdinal >= 0 && podOrdinal < ingGrayIndex {
		gray.Type = ingress.ActiveGray
	} else {
		gray.Type = ingress.InactiveGray
	}

	klog.Infof("Get ingress %v status {Gray[type:%v],IngressGrayFlag[%v],IngressGrayCurVer[%v],IngressGrayNewVer[%v],IngressGrayIndex[%v],PodOrdinal[%v]}",
		key, gray.Type, anns.IngGray.IngGrayFlag, anns.IngGray.IngGrayCurVer, anns.IngGray.IngGrayNewVer, anns.IngGray.IngGrayIndex, podOrdinal)

	return gray, nil
}

// GetSecretGrayStatus returns gray status of a secret
func (s *k8sStore) GetSecretGrayStatus(key string) (ingress.SecretGray, error) {
	gray := ingress.SecretGray{
		Type: ingress.Active,
	}

	secret, err := s.listers.SecretWithAnnotation.ByKey(key)
	if err != nil {
		klog.Errorf("get a secret [%v] with annotations in the store failed [%v]", key, err)
		return gray, err
	}

	anns := secret.ParsedAnnotations
	podOrdinal := astsutils.GetPodOrdinal(s.pod)
	secGrayIndex := int32(anns.SecretGray.SecGrayIndex)
	if !anns.SecretGray.SecGrayFlag {
		gray.Type = ingress.Active
	} else if secGrayIndex > 0 && podOrdinal >= 0 && podOrdinal < secGrayIndex {
		gray.Type = ingress.ActiveGray
	} else {
		gray.Type = ingress.InactiveGray
	}

	klog.Infof("Get secret %v status {Gray[type:%v],SecretGrayFlag[%v],SecretGrayCurVer[%v],SecretGrayNewVer[%v],SecretGrayIndex[%v],PodOrdinal[%v]}",
		key, gray.Type, anns.SecretGray.SecGrayFlag, anns.SecretGray.SecGrayCurVer, anns.SecretGray.SecGrayNewVer, anns.SecretGray.SecGrayIndex, podOrdinal)

	return gray, nil
}

// ListSecretsWithAnnotation returns the list of Secrets with annotations
func (s *k8sStore) ListSecretsWithAnnotation() []*ingress.Secret {
	// filter secret rules
	secrets := make([]*ingress.Secret, 0)
	for _, item := range s.listers.SecretWithAnnotation.List() {
		secret := item.(*ingress.Secret)
		secrets = append(secrets, secret)
	}

	// sort Secrets using the CreationTimestamp field
	sort.SliceStable(secrets, func(i, j int) bool {
		ir := secrets[i].CreationTimestamp
		jr := secrets[j].CreationTimestamp
		if ir.Equal(&jr) {
			in := fmt.Sprintf("%v/%v", secrets[i].Namespace, secrets[i].Name)
			jn := fmt.Sprintf("%v/%v", secrets[j].Namespace, secrets[j].Name)
			klog.V(3).Infof("Secret %v and %v have identical CreationTimestamp", in, jn)
			return in > jn
		}
		return ir.Before(&jr)
	})

	return secrets
}

// ListLocalSecretCheckSums returns a list of local SecretCheckSums.
func (s *k8sStore) ListLocalSecretCheckSums(filter SecretCheckFilterFunc) []*secretcheckv1.SecretCheckSum {
	// filter SecretCheckSum
	var secretCheckSums []*secretcheckv1.SecretCheckSum
	for _, item := range s.secretCheckSumStore.List() {
		secretCheckSum := item.(*secretcheckv1.SecretCheckSum)
		if filter != nil && filter(secretCheckSum) {
			continue
		}

		secretCheckSums = append(secretCheckSums, secretCheckSum)
	}

	// reverse-order SecretCheckSum using the CreationTimestamp field
	sort.SliceStable(secretCheckSums, func(i, j int) bool {
		ir := secretCheckSums[i].CreationTimestamp
		jr := secretCheckSums[j].CreationTimestamp
		if ir.Equal(&jr) {
			in := fmt.Sprintf("%v/%v", secretCheckSums[i].Namespace, secretCheckSums[i].Name)
			jn := fmt.Sprintf("%v/%v", secretCheckSums[j].Namespace, secretCheckSums[j].Name)
			klog.V(3).Infof("IngressCheckSum %v and %v have identical CreationTimestamp", in, jn)
			return in < jn
		}
		return !ir.Before(&jr)
	})

	return secretCheckSums
}

// GetSecretCheckSum returns the SecretCheckSum matching key.
func (s *k8sStore) GetSecretCheckSum(key string) (*secretcheckv1.SecretCheckSum, error) {
	return s.listers.SecretCheckSum.ByKey(key)
}

// GetLocalSecretCheckSum returns the local cocy of a SecretCheckSum matching key.
func (s *k8sStore) GetLocalSecretCheckSum(key string) (*secretcheckv1.SecretCheckSum, error) {
	return s.secretCheckSumStore.ByKey(key)
}

// getIngInformer returns the ingress informer.
func (s *k8sStore) getIngInformer(useStorageCluster bool, infFactory informers.SharedInformerFactory, ingFactory informers.SharedInformerFactory) cache.SharedIndexInformer {
	var ingInformer cache.SharedIndexInformer
	if useStorageCluster {
		ingInformer = ingFactory.Networking().V1().Ingresses().Informer()

	} else {
		ingInformer = infFactory.Networking().V1().Ingresses().Informer()
	}

	return ingInformer
}

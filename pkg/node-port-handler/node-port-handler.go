package nodeporthandler

import (
	"encoding/json"
	"fmt"
	"time"

	routeshandler "github.com/crc-org/routes-controller/pkg/routes-handler"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	vmVirtualIP = "192.168.127.2"
	localhost   = "127.0.0.1"
)

func NodePortHandler(clientset *kubernetes.Clientset) cache.SharedIndexInformer {
	factory := informers.NewSharedInformerFactory(clientset, 5*time.Minute)
	informer := factory.Core().V1().Services().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			log.Infof("Added service '%s' of type '%s'", service.GetName(), service.Spec.Type)
			if service.Spec.Type != v1.ServiceTypeNodePort {
				log.Debugf("Service '%s' is not of type NodePort. Nothing to do.", service.GetName())
				return
			}

			for _, port := range service.Spec.Ports {
				if err := expose(port); err != nil {
					log.Error(err)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldService := oldObj.(*v1.Service)
			newService := newObj.(*v1.Service)

			log.Infof("Service '%s' of type '%s' changed!", newService.GetName(), oldService.Spec.Type)

			if oldService.Spec.Type == v1.ServiceTypeNodePort {
				log.Debugf("Unexposing previous NodePorts...")
				for _, port := range oldService.Spec.Ports {
					if err := unexpose(port); err != nil {
						log.Error(err)
					}
				}
			}

			if newService.Spec.Type == v1.ServiceTypeNodePort {
				log.Debugf("Exposing current NodePorts...")
				for _, port := range newService.Spec.Ports {
					if err := expose(port); err != nil {
						log.Error(err)
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*v1.Service)
			log.Infof("Deleted service '%s' of type '%s'", service.GetName(), service.Spec.Type)
			if service.Spec.Type != v1.ServiceTypeNodePort {
				log.Debugf("Service '%s' is not of type NodePort. Nothing to do.", service.GetName())
				return
			}

			for _, port := range service.Spec.Ports {
				if err := unexpose(port); err != nil {
					log.Error(err)
				}
			}
		},
	})
	if err != nil {
		log.Errorf("failed to add event handler: %v", err)
	}

	return informer
}

type ExposeRequest struct {
	Local  string
	Remote string
}

type UnexposeRequest struct {
	Local string
}

var exposeURL = "http://host/services/forwarder/expose"

var unexposeURL = "http://host/services/forwarder/unexpose"

func expose(port v1.ServicePort) error {
	if port.Protocol != v1.ProtocolTCP {
		log.Infof("Skipping NodePort %d: protocol %s is not supported", port.NodePort, port.Protocol)
		return nil
	}
	exposeRequest := ExposeRequest{
		Local:  fmt.Sprintf("%s:%d", localhost, port.NodePort),
		Remote: fmt.Sprintf("%s:%d", vmVirtualIP, port.NodePort),
	}
	log.Infof("Exposing port (%s -> %s)", exposeRequest.Local, exposeRequest.Remote)

	bin, err := json.Marshal(exposeRequest)
	if err != nil {
		return err
	}

	return routeshandler.PostJSON(exposeURL, bin)
}

func unexpose(port v1.ServicePort) error {
	if port.Protocol != v1.ProtocolTCP {
		log.Infof("Skipping NodePort %d: protocol %s is not supported", port.NodePort, port.Protocol)
		return nil
	}
	unexposeRequest := UnexposeRequest{
		Local: fmt.Sprintf("%s:%d", localhost, port.NodePort),
	}
	log.Infof("Unexposing port '%s'", unexposeRequest.Local)

	bin, err := json.Marshal(unexposeRequest)
	if err != nil {
		return err
	}

	return routeshandler.PostJSON(unexposeURL, bin)
}

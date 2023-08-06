package context

import "github.com/ahmetson/service-lib/config/service"

type Type = string

// DevContext indicates that all dependency proxies are in the local machine
const DevContext Type = "development"

// DefaultContext indicates that all dependencies are in any machine.
// It's unspecified.
const DefaultContext Type = "default"

type Interface interface {
	ReadService(string) (*service.Service, error) // string arg is the service url
	WriteService(string, *service.Service) error  // string arg is service url
	Paths() []string
	SetUrl(url string)
	GetUrl() string
	Host() string
	GetType() Type
}
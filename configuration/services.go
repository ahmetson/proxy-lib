package configuration

import (
	"fmt"
)

type ControllerInstance struct {
	Port     uint64
	Instance string
}

type Controller struct {
	Type      Type
	Name      string
	Instances []ControllerInstance
}

type Proxy struct {
	Name string
	Port uint64
}

type Extension struct {
	Name string
	Port uint64
}

// Service type defined in the configuration
type Service struct {
	Type        ServiceType
	Name        string
	Instance    string
	Controllers []Controller
	Proxies     []Proxy
	Extensions  []Extension
	Pipelines   []string
}

// Validate the parameters of the service
func (s *Service) Validate() error {
	if err := ValidateServiceType(s.Type); err != nil {
		return fmt.Errorf("identity.ValidateServiceType: %v", err)
	}

	for _, c := range s.Controllers {
		if err := ValidateControllerType(c.Type); err != nil {
			return fmt.Errorf("controller.ValidateControllerType: %v", err)
		}
	}

	return nil
}

// GetController returns the controller configuration by the controller name.
// If the controller doesn't exist, then it returns an error.
func (s *Service) GetController(name string) (*Controller, error) {
	for _, c := range s.Controllers {
		if c.Name == name {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("'%s' controller was not found in '%s' service's configuration", name, s.Name)
}

// GetFirstController returns the controller without requiring its name.
// If the service doesn't have controllers, then it will return an error.
func (s *Service) GetFirstController() (*Controller, error) {
	if len(s.Controllers) == 0 {
		return nil, fmt.Errorf("service '%s' doesn't have any controllers in yaml file", s.Name)
	}

	controller := s.Controllers[0]
	return &controller, nil
}

// GetExtension returns the extension configuration by the extension name.
// If the extension doesn't exist, then it returns an error.
func (s *Service) GetExtension(name string) (*Extension, error) {
	for _, e := range s.Extensions {
		if e.Name == name {
			return &e, nil
		}
	}

	return nil, fmt.Errorf("'%s' extension was not found in '%s' service's configuration", name, s.Name)
}

type Services []Service
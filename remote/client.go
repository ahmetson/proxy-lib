// Package remote defines client socket that can access to the remote service.
package remote

import (
	"fmt"
	"github.com/Seascape-Foundation/sds-service-lib/remote/parameter"

	"github.com/Seascape-Foundation/sds-common-lib/data_type/key_value"
	"github.com/Seascape-Foundation/sds-service-lib/communication/message"
	"github.com/Seascape-Foundation/sds-service-lib/configuration"
	service "github.com/Seascape-Foundation/sds-service-lib/identity"
	"github.com/Seascape-Foundation/sds-service-lib/log"

	// todo
	// move out dependency from security/auth
	// "github.com/Seascape-Foundation/sds-service-lib/security/auth"
	zmq "github.com/pebbe/zmq4"
)

// ClientSocket is the wrapper around zeromq's socket.
// The socket is the client's socket that will try to interact with the remote service.
type ClientSocket struct {
	// The name of remote SDS service and its URL
	// its used as a clarification
	remoteService *service.Service
	// client_credentials *auth.Credentials
	serverPublicKey string
	poller          *zmq.Poller
	socket          *zmq.Socket
	protocol        string
	inprocUrl       string
	logger          log.Logger
	appConfig       *configuration.Config
	serviceName     string
	servicePort     uint64
}

// Clients is the key value but with the additional functions
// to cast the interface{} to ClientSocket
type Clients = key_value.KeyValue

// AddClient Adds the client to the list of the clients
func AddClient(clients Clients, name string, socket *ClientSocket) {
	clients.Set(name, socket)
}

// ClientExist checks whether the client exists or not
func ClientExist(clients Clients, name string) bool {
	return clients.Exist(name) == nil
}

// GetClient returns the client from the list
func GetClient(clients Clients, name string) *ClientSocket {
	kv := clients.Map()
	return kv[name].(*ClientSocket)
}

// Initiates the socket with a timeout.
// If the socket is already given, then reconnect() closes it.
// Then creates a new socket.
//
// If no socket is given, then initiates a zmq.REQ socket.
func (socket *ClientSocket) reconnect() error {
	var socketCtx *zmq.Context
	var socketType zmq.Type

	if socket.socket != nil {
		ctx, err := socket.socket.Context()
		if err != nil {
			return fmt.Errorf("failed to get context from zmq socket: %w", err)
		} else {
			socketCtx = ctx
		}

		socketType, err = socket.socket.GetType()
		if err != nil {
			return fmt.Errorf("failed to get socket type from zmq socket: %w", err)
		}

		err = socket.Close()
		if err != nil {
			return fmt.Errorf("failed to close socket in zmq: %w", err)
		}
		socket.socket = nil
	} else {
		return fmt.Errorf("no socket initiated: %s", "reconnect")
	}

	sock, err := socketCtx.NewSocket(socketType)
	if err != nil {
		return fmt.Errorf("failed to create %s socket: %w", socketType.String(), err)
	} else {
		socket.socket = sock
		err = socket.socket.SetLinger(0)
		if err != nil {
			return fmt.Errorf("failed to set up linger parameter for zmq socket: %w", err)
		}
	}

	// if socket.client_credentials != nil {
	// socket.client_credentials.SetClientAuthCurve(socket.socket, socket.server_public_key)
	// if err != nil {
	// return fmt.Errorf("auth.SetClientAuthCurve: %w", err)
	// }
	// }

	if err := socket.socket.Connect(socket.remoteService.Url()); err != nil {
		return fmt.Errorf("socket connect: %w", err)
	}

	socket.poller = zmq.NewPoller()
	socket.poller.Add(socket.socket, zmq.POLLIN)

	return nil
}

// Attempts to connect to the endpoint.
// The difference from socket.reconnect() is that it will not authenticate if security is enabled.
func (socket *ClientSocket) inprocReconnect() error {
	var socketCtx *zmq.Context
	var socketType zmq.Type

	if socket.socket != nil {
		ctx, err := socket.socket.Context()
		if err != nil {
			return fmt.Errorf("failed to get context from zmq socket: %w", err)
		} else {
			socketCtx = ctx
		}

		socketType, err = socket.socket.GetType()
		if err != nil {
			return fmt.Errorf("failed to get socket type from zmq socket: %w", err)
		}

		err = socket.Close()
		if err != nil {
			return fmt.Errorf("failed to close socket in zmq: %w", err)
		}
		socket.socket = nil
	} else {
		return fmt.Errorf("failed to create zmq context: %s", "inproc_reconnect")
	}

	sock, err := socketCtx.NewSocket(socketType)
	if err != nil {
		return fmt.Errorf("failed to create %s socket: %w", socketType.String(), err)
	} else {
		socket.socket = sock
		err = socket.socket.SetLinger(0)
		if err != nil {
			return fmt.Errorf("failed to set up linger parameter for zmq socket: %w", err)
		}
	}

	if err := socket.socket.Connect(socket.inprocUrl); err != nil {
		return fmt.Errorf("error '%s' connect: %w", socket.inprocUrl, err)
	}

	socket.poller = zmq.NewPoller()
	socket.poller.Add(socket.socket, zmq.POLLIN)

	return nil
}

// Close the socket free the port and resources.
func (socket *ClientSocket) Close() error {
	err := socket.socket.Close()
	if err != nil {
		return fmt.Errorf("error closing socket: %w", err)
	}

	return nil
}

// RequestRouter sends request message to the router. Then router will redirect it to the controller
// defined in the service_type.
//
// Supports both inproc and TCP protocols.
//
// The socket should be the router's socket.
func (socket *ClientSocket) RequestRouter(service *service.Service, request *message.Request) (key_value.KeyValue, error) {
	requestTimeout := parameter.RequestTimeout(socket.appConfig)

	if socket.protocol == "inproc" {
		err := socket.inprocReconnect()
		if err != nil {
			return nil, fmt.Errorf("inproc_reconnect: %w", err)
		}

	} else {
		err := socket.reconnect()
		if err != nil {
			return nil, fmt.Errorf("reconnect: %w", err)
		}
	}

	requestString, err := request.ToString()
	if err != nil {
		return nil, fmt.Errorf("request.ToString: %w", err)
	}

	attempt := parameter.Attempt(socket.appConfig)

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.socket.SendMessage(service.Name, requestString); err != nil {
			return nil, fmt.Errorf("failed to send the command '%s' to. socket error: %w", request.Command, err)
		}

		//  Poll socket for a reply, with timeout
		sockets, err := socket.poller.Poll(requestTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to to send the command '%s'. poll error: %w", request.Command, err)
		}

		//  Here we process a server reply and exit our loop if the
		//  reply is valid. If we didn't a reply we close the client
		//  socket and resend the request. We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for reply.
			r, err := socket.socket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to receive the command '%s' message. socket error: %w", request.Command, err)
			}

			reply, err := message.ParseReply(r)
			if err != nil {
				return nil, fmt.Errorf("failed to parse the command '%s': %w", request.Command, err)
			}

			if !reply.IsOK() {
				return nil, fmt.Errorf("the command '%s' replied with a failure: %s", request.Command, reply.Message)
			}

			return reply.Parameters, nil
		} else {
			socket.logger.Warn("Timeout! Are you sure that remote service is running?", "target service name", service.Name, "request_command", request.Command, "attempts_left", attempt, "request_timeout", requestTimeout)
			// if attempts are 0, we reconnect to remove the buffer queue.
			if socket.protocol == "inproc" {
				err := socket.inprocReconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.inproc_reconnect: %w", err)
				}
			} else {
				err := socket.reconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.reconnect: %w", err)
				}
			}
			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--
		}
	}
}

// RequestRemoteService sends the request message to the socket.
// Returns the message.Reply.Parameters in case of success.
//
// Error is returned in other cases.
//
// If the remote service returned failure message its converted into an error.
//
// The socket type should be REQ or PUSH.
func (socket *ClientSocket) RequestRemoteService(request *message.Request) (key_value.KeyValue, error) {
	if socket.protocol == "inproc" {
		err := socket.inprocReconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}

	} else {
		err := socket.reconnect()
		if err != nil {
			return nil, fmt.Errorf("socket connection: %w", err)
		}
	}

	requestTimeout := parameter.RequestTimeout(socket.appConfig)

	requestString, err := request.ToString()
	if err != nil {
		return nil, fmt.Errorf("request.ToString: %w", err)
	}

	attempt := parameter.Attempt(socket.appConfig)

	// we attempt requests for an infinite amount of time.
	for {
		//  We send a request, then we work to get a reply
		if _, err := socket.socket.SendMessage(requestString); err != nil {
			return nil, fmt.Errorf("failed to send the command '%s'. socket error: %w", request.Command, err)
		}

		//  Poll socket for a reply, with timeout
		sockets, err := socket.poller.Poll(requestTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to to send the command '%s'. poll error: %w", request.Command, err)
		}

		//  Here we process a server reply and exit our loop if the
		//  reply is valid. If we didn't a reply we close the client
		//  socket and resend the request. We try a number of times
		//  before finally abandoning:

		if len(sockets) > 0 {
			// Wait for reply.
			r, err := socket.socket.RecvMessage(0)
			if err != nil {
				return nil, fmt.Errorf("failed to receive the command '%s' message. socket error: %w", request.Command, err)
			}

			reply, err := message.ParseReply(r)
			if err != nil {
				return nil, fmt.Errorf("failed to parse the command '%s': %w", request.Command, err)
			}

			if !reply.IsOK() {
				return nil, fmt.Errorf("the command '%s' replied with a failure: %s", request.Command, reply.Message)
			}

			return reply.Parameters, nil
		} else {
			socket.logger.Warn("timeout", "request_command", request.Command, "attempts_left", attempt)
			if socket.protocol == "inproc" {
				err := socket.inprocReconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.inproc_reconnect: %w", err)
				}
			} else {
				err := socket.reconnect()
				if err != nil {
					return nil, fmt.Errorf("socket.reconnect: %w", err)
				}
			}

			if attempt == 0 {
				return nil, fmt.Errorf("timeout")
			}
			attempt--
		}
	}
}

// // SetSecurity will set the curve credentials in the socket to authenticate with the remote service.
// func (socket *ClientSocket) SetSecurity(server_public_key string, client *auth.Credentials) *ClientSocket {
// 	socket.server_public_key = server_public_key
// 	socket.client_credentials = client

// 	return socket
// }

// NewTcpSocket creates a new client socket over TCP protocol.
//
// The returned socket client then can send message to controller.Router and controller.Reply
func NewTcpSocket(remoteService *service.Service, parent *log.Logger, appConfig *configuration.Config) (*ClientSocket, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("missing app_config")
	}

	if remoteService == nil ||
		remoteService.IsInproc() ||
		!remoteService.IsRemote() {
		return nil, fmt.Errorf("remote service is not a remote service with REMOTE limit")
	}

	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	var logger log.Logger
	if parent != nil {
		logger, err = parent.Child("client_socket",
			"remote_service", remoteService.Name,
			"protocol", "tcp",
			"socket_type", "REQ",
			"remote_service_url", remoteService.Url(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		remoteService: remoteService,
		// client_credentials: nil,
		socket:    sock,
		protocol:  "tcp",
		logger:    logger,
		appConfig: appConfig,
	}

	return &newSocket, nil
}

// NewReq creates a new client to connect to the service labelled as name.
//
// The returned socket client then can send message to the controller.Replier
func NewReq(name string, port uint64, parent *log.Logger) (*ClientSocket, error) {
	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	var logger log.Logger
	if parent != nil {
		logger, err = parent.Child("client",
			"service name", name,
			"protocol", "tcp",
			"socket_type", "REQ",
			"remote_service_url", ClientUrl(port),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		remoteService: nil,
		socket:        sock,
		protocol:      "tcp",
		logger:        logger,
		appConfig:     nil,
		serviceName:   name,
		servicePort:   port,
	}

	return &newSocket, nil
}

// InprocRequestSocket creates a client socket with inproc protocol.
// The created client socket can connect to controller.Router or controller.Reply.
//
// The `url` parameter must start with `inproc://`
func InprocRequestSocket(url string, parent log.Logger, appConfig *configuration.Config) (*ClientSocket, error) {
	if appConfig == nil {
		return nil, fmt.Errorf("missing app_config")
	}

	if len(url) < 9 {
		return nil, fmt.Errorf("the url is too short")
	}
	if url[:9] != "inproc://" {
		return nil, fmt.Errorf("url doesn't start with `inproc` protocol. Its %s", url[:9])
	}

	sock, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		return nil, fmt.Errorf("zmq.NewSocket: %w", err)
	}

	logger, err := parent.Child("client_socket",
		"protocol", "inproc",
		"socket_type", "REQ",
		"remote_service_url", url)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	newSocket := ClientSocket{
		socket:    sock,
		protocol:  "inproc",
		inprocUrl: url,
		// client_credentials: nil,
		logger:    logger,
		appConfig: appConfig,
	}

	return &newSocket, nil
}

// // NewTcpSubscriber create a new client socket on TCP protocol.
// // The created client can subscribe to broadcast.Broadcast
// func NewTcpSubscriber(e *service.Service, server_public_key string, client *auth.Credentials, parent log.Logger, app_config *configuration.Config) (*ClientSocket, error) {
// 	if app_config == nil {
// 		return nil, fmt.Errorf("missing app_config")
// 	}
// 	if e == nil {
// 		return nil, fmt.Errorf("missing service")
// 	}
// 	if !e.IsSubscribe() || e.IsInproc() {
// 		return nil, fmt.Errorf("the service is of tcp protocol, or it doesn't SUBSCRIBE limit")
// 	}

// 	socket, sockErr := zmq.NewSocket(zmq.SUB)
// 	if sockErr != nil {
// 		return nil, fmt.Errorf("new sub socket: %w", sockErr)
// 	}

// 	if client != nil {
// 		err := client.SetClientAuthCurve(socket, server_public_key)
// 		if err != nil {
// 			return nil, fmt.Errorf("client.SetClientAuthCurve: %w", err)
// 		}
// 	}

// 	conErr := socket.Connect(e.Url())
// 	if conErr != nil {
// 		return nil, fmt.Errorf("connect to broadcast: %w", conErr)
// 	}

// 	logger, err := parent.Child("client_socket",
// 		"remote_service", e.Name,
// 		"protocol", "tcp",
// 		"socket_type", "Subscriber",
// 		"remote_service_url", e.Url(),
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("logger: %w", err)
// 	}

// 	return &ClientSocket{
// 		remote_service:     e,
// 		socket:             socket,
// 		client_credentials: client,
// 		server_public_key:  server_public_key,
// 		protocol:           "tcp",
// 		logger:             logger,
// 		app_config:         app_config,
// 	}, nil
// }

// ClientUrl creates url of the controller for the client to connect
func ClientUrl(port uint64) string {
	url := fmt.Sprintf("tcp://localhost:%d", port)
	return url
}
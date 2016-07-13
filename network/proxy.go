package network

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/subgraph/oz/ns"

	"github.com/op/go-logging"
)

type ProxyType string

const (
	PROXY_CLIENT ProxyType = "client"
	PROXY_SERVER ProxyType = "server"
)

type ProtoType string

const (
	PROTO_TCP        ProtoType = "tcp"
	PROTO_UDP        ProtoType = "udp"
	PROTO_UNIX       ProtoType = "unix"
	PROTO_UNIXGRAM   ProtoType = "unixgram"
	PROTO_UNIXPACKET ProtoType = "unixpacket"
)

// Socket list, used to hold ports that should be forwarded
type ProxyConfig struct {
	// One of client, server
	Nettype ProxyType `json:"type"`

	// One of tcp, udp, socket
	Proto ProtoType

	// TCP or UDP port number
	Port int

	// Optional: Destination address
	// In client mode: the host side address to connect to
	// In server mode: the sandbox side address to bind to
	// For unix sockets this is an abstract path
	// If left empty, localhost is used
	Destination string
}

var wgProxy sync.WaitGroup

func ProxySetup(childPid int, ozSockets []ProxyConfig, log *logging.Logger, ready sync.WaitGroup) error {
	for _, socket := range ozSockets {
		if socket.Nettype == "" {
			continue
		}
		if socket.Nettype == PROXY_CLIENT {
			err := newProxyClient(childPid, &socket, log, ready)
			if err != nil {
				return fmt.Errorf("%+v, %s", socket, err)
			}
		} else if socket.Nettype == PROXY_SERVER {
			err := newProxyServer(childPid, &socket, log, ready)
			if err != nil {
				return fmt.Errorf("%+s, %s", socket, err)
			}
		}
	}

	return nil
}

/**
 * Listener/Client
**/
func proxyClientConn(conn *net.Conn, proto ProtoType, rAddr string, ready sync.WaitGroup) error {
	rConn, err := net.Dial(string(proto), rAddr)
	if err != nil {
		return fmt.Errorf("Socket: %+v.\n", err)
	}

	go io.Copy(rConn, *conn)
	go io.Copy(*conn, rConn)

	return nil
}

func newProxyClient(pid int, config *ProxyConfig, log *logging.Logger, ready sync.WaitGroup) error {
	if config.Destination == "" {
		config.Destination = "127.0.0.1"
	}

	var lAddr, rAddr string
	if !strings.HasPrefix(string(config.Proto), "unix") {
		lAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(config.Port))
		rAddr = net.JoinHostPort(config.Destination, strconv.Itoa(config.Port))
	} else {
		lAddr = fmt.Sprintf("@%s", config.Destination)
		rAddr = config.Destination
	}

	log.Info("Starting socket client forwarding: %s://%s --> %s://%s.", config.Proto, lAddr, config.Proto, rAddr)

	listen, err := proxySocketListener(pid, config.Proto, lAddr)
	if err != nil {
		return err
	}

	wgProxy.Add(1)
	go func() {
		defer wgProxy.Done()
		for {
			conn, err := listen.Accept()
			if err != nil {
				log.Error("Socket: %+v.", err)
				//panic(err)
				continue
			}

			go proxyClientConn(&conn, config.Proto, rAddr, ready)
		}
	}()

	return nil
}

func proxySocketListener(pid int, proto ProtoType, lAddr string) (net.Listener, error) {
	fd, err := ns.OpenProcess(pid, ns.CLONE_NEWNET)
	defer ns.Close(fd)
	if err != nil {
		return nil, err
	}

	return nsSocketListener(fd, proto, lAddr)
}

func nsSocketListener(fd uintptr, proto ProtoType, lAddr string) (net.Listener, error) {
	origNs, _ := ns.OpenProcess(os.Getpid(), ns.CLONE_NEWNET)
	defer ns.Close(origNs)
	defer ns.Set(origNs, ns.CLONE_NEWNET)

	err := ns.Set(uintptr(fd), ns.CLONE_NEWNET)
	if err != nil {
		return nil, err
	}

	return net.Listen(string(proto), lAddr)

}

/**
 * Connect/Server
**/
func proxyServerConn(pid int, conn *net.Conn, proto ProtoType, rAddr string, log *logging.Logger, ready sync.WaitGroup) error {
	rConn, err := socketConnect(pid, proto, rAddr)
	if err != nil {
		log.Error("Socket: %+v.", err)
		return err
	}

	go io.Copy(*conn, rConn)
	go io.Copy(rConn, *conn)

	return nil
}

func newProxyServer(pid int, config *ProxyConfig, log *logging.Logger, ready sync.WaitGroup) error {
	if config.Destination == "" {
		config.Destination = "127.0.0.1"
	}

	var lAddr, rAddr string
	if !strings.HasPrefix(string(config.Proto), "unix") {
		lAddr = net.JoinHostPort(config.Destination, strconv.Itoa(config.Port))
		rAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(config.Port))
	} else {
		lAddr = config.Destination
		rAddr = config.Destination
	}

	log.Info("Starting socket server forwarding: %s://%s.", config.Proto, lAddr)

	listen, err := net.Listen(string(config.Proto), lAddr)
	if err != nil {
		return err
	}

	wgProxy.Add(1)
	go func() {
		defer wgProxy.Done()
		for {
			conn, err := listen.Accept()
			if err != nil {
				log.Error("Socket: %+v.", err)
				//panic(err)
				continue
			}

			go proxyServerConn(pid, &conn, config.Proto, rAddr, log, ready)
		}
	}()

	return nil
}

func socketConnect(pid int, proto ProtoType, rAddr string) (net.Conn, error) {
	fd, err := ns.OpenProcess(pid, ns.CLONE_NEWNET)
	defer ns.Close(fd)
	if err != nil {
		return nil, err
	}

	return nsProxySocketConnect(fd, proto, rAddr)
}

func nsProxySocketConnect(fd uintptr, proto ProtoType, rAddr string) (net.Conn, error) {
	origNs, _ := ns.OpenProcess(os.Getpid(), ns.CLONE_NEWNET)
	defer ns.Close(origNs)
	defer ns.Set(origNs, ns.CLONE_NEWNET)

	err := ns.Set(uintptr(fd), ns.CLONE_NEWNET)
	if err != nil {
		return nil, err
	}

	return net.Dial(string(proto), rAddr)

}

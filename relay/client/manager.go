package client

import (
	"context"
	"fmt"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
)

type RelayTrack struct {
	sync.RWMutex
	relayClient *Client
}

func NewRelayTrack() *RelayTrack {
	return &RelayTrack{}
}

type Manager struct {
	ctx        context.Context
	srvAddress string
	peerID     string

	relayClient    *Client
	reconnectGuard *Guard

	relayClients      map[string]*RelayTrack
	relayClientsMutex sync.RWMutex
}

func NewManager(ctx context.Context, serverAddress string, peerID string) *Manager {
	return &Manager{
		ctx:          ctx,
		srvAddress:   serverAddress,
		peerID:       peerID,
		relayClients: make(map[string]*RelayTrack),
	}
}

func (m *Manager) Serve() error {
	m.relayClient = NewClient(m.ctx, m.srvAddress, m.peerID)
	m.reconnectGuard = NewGuard(m.ctx, m.relayClient)
	m.relayClient.SetOnDisconnectListener(m.reconnectGuard.OnDisconnected)
	err := m.relayClient.Connect()
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) OpenConn(serverAddress, peerKey string) (net.Conn, error) {
	if m.relayClient == nil {
		return nil, fmt.Errorf("relay client not connected")
	}

	foreign, err := m.isForeignServer(serverAddress)
	if err != nil {
		return nil, err
	}

	if !foreign {
		return m.relayClient.OpenConn(peerKey)
	} else {
		return m.openConnVia(serverAddress, peerKey)
	}
}

func (m *Manager) RelayAddress() (net.Addr, error) {
	if m.relayClient == nil {
		return nil, fmt.Errorf("relay client not connected")
	}
	return m.relayClient.RelayRemoteAddress()
}

func (m *Manager) openConnVia(serverAddress, peerKey string) (net.Conn, error) {
	m.relayClientsMutex.RLock()
	relayTrack, ok := m.relayClients[serverAddress]
	if ok {
		relayTrack.RLock()
		m.relayClientsMutex.RUnlock()
		defer relayTrack.RUnlock()
		return relayTrack.relayClient.OpenConn(peerKey)
	}
	m.relayClientsMutex.RUnlock()

	rt := NewRelayTrack()
	rt.Lock()

	m.relayClientsMutex.Lock()
	m.relayClients[serverAddress] = rt
	m.relayClientsMutex.Unlock()

	relayClient := NewClient(m.ctx, serverAddress, m.peerID)
	err := relayClient.Connect()
	if err != nil {
		rt.Unlock()
		m.relayClientsMutex.Lock()
		delete(m.relayClients, serverAddress)
		m.relayClientsMutex.Unlock()
		return nil, err
	}
	relayClient.SetOnDisconnectListener(func() {
		m.deleteRelayConn(serverAddress)
	})
	rt.Unlock()

	conn, err := relayClient.OpenConn(peerKey)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (m *Manager) deleteRelayConn(address string) {
	log.Infof("deleting relay client for %s", address)
	m.relayClientsMutex.Lock()
	defer m.relayClientsMutex.Unlock()

	delete(m.relayClients, address)
}

func (m *Manager) isForeignServer(address string) (bool, error) {
	rAddr, err := m.relayClient.RelayRemoteAddress()
	if err != nil {
		return false, fmt.Errorf("relay client not connected")
	}
	return rAddr.String() != address, nil
}

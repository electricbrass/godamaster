// Copyright (C) 2025 Mia McMahill
//
// This program is free software; you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

package serverlist

import (
	"errors"
	"iter"
	"maps"
	"net"
	"time"
)

const MAX_SERVERS = 1024
const MAX_SERVERS_PER_IP = 64

type Player struct {
	Name  string
	Frags int16
	Ping  int32
	Team  byte
}

type Server struct {
	Addr net.UDPAddr
	Age  time.Time

	// server data
	Hostname   string
	Numplayers byte
	Maxplayers byte
	Curmap     string
	Pwads      []string
	Gametype   byte
	Skill      byte
	Teamplay   byte
	Ctfmode    byte
	Players    []Player

	KeySent  uint32
	Verified bool
	Pinged   bool
}

type ServerList struct {
	servers   map[string]*Server
	addresses map[string]map[int]*Server
}

func New() *ServerList {
	return &ServerList{
		servers:   make(map[string]*Server),
		addresses: make(map[string]map[int]*Server),
	}
}

func (list *ServerList) Servers() iter.Seq[*Server] {
	return maps.Values(list.servers)
}

func (list *ServerList) AddServer(server *Server) error {
	if list.ReachedMaxServers() {
		return errors.New("Max servers reached")
	}
	if list.ReachedMaxServersForIP(&server.Addr.IP) {
		return errors.New("Max servers per IP reached")
	}

	ip := server.Addr.IP.String()
	list.servers[server.Addr.String()] = server
	if list.addresses[ip] == nil {
		list.addresses[ip] = make(map[int]*Server)
	}
	list.addresses[ip][server.Addr.Port] = server

	return nil
}

func (list *ServerList) RemoveServer(server *Server) {
	delete(list.servers, server.Addr.String())
	ip := server.Addr.IP.String()
	delete(list.addresses[ip], server.Addr.Port)
	if len(list.addresses[ip]) == 0 {
		delete(list.addresses, ip)
	}
}

func (list *ServerList) GetServer(addr *net.UDPAddr) *Server {
	return list.servers[addr.String()]
}

func (list *ServerList) Size() int {
	return len(list.servers)
}

func (list *ServerList) ReachedMaxServers() bool {
	return len(list.servers) >= MAX_SERVERS
}

func (list *ServerList) ReachedMaxServersForIP(ip *net.IP) bool {
	if list.addresses[ip.String()] == nil {
		return false
	}
	return len(list.addresses[ip.String()]) >= MAX_SERVERS_PER_IP
}

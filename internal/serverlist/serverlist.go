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
	"encoding/csv"
	"errors"
	"fmt"
	"iter"
	"maps"
	"net"
	"os"
	"strings"
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
		return fmt.Errorf("Max servers reached for IP %v", server.Addr.IP)
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

func (list *ServerList) ToCSV(filepath string) error {
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	headers := []string{"Name", "Map", "Players/Max", "WADs", "Gametype", "Address:Port"}
	err = w.Write(headers)
	if err != nil {
		return err
	}

	for _, server := range list.servers {
		if !server.Verified {
			continue
		}
		var gametype string
		switch {
		case server.Ctfmode == 1:
			gametype = "CTF"
		case server.Gametype == 1 && server.Teamplay == 1:
			gametype = "TEAM DM"
		case server.Gametype == 0:
			gametype = "COOP"
		default:
			gametype = "DM"
		}

		pwads := strings.Join(server.Pwads, " ")
		if pwads == "" {
			pwads = " "
		}

		row := []string{
			server.Hostname,
			server.Curmap,
			fmt.Sprintf("%d/%d", server.Numplayers, server.Maxplayers),
			pwads,
			gametype,
			fmt.Sprintf("%s:%d", server.Addr.IP.String(), server.Addr.Port),
		}

		err = w.Write(row)
		if err != nil {
			return err
		}
	}

	return nil
}

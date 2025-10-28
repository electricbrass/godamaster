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

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"time"

	"github.com/electricbrass/godamaster/internal/serverlist"
)

const MASTER_PORT = 15000

const MAX_SERVER_AGE = time.Minute * 5
const MAX_UNVERIFIED_SERVER_AGE = time.Minute

const SERVER_CHALLENGE = 5560020
const LAUNCHER_CHALLENGE = 777123

const MAX_UDP_PACKET_SIZE = 1400

var serverList *serverlist.ServerList

func addServer(addr *net.UDPAddr) {
	server := serverList.GetServer(addr)

	// server already exists?
	if server != nil {
		server.Age = time.Now()
		server.Pinged = false
		slog.Debug("Refreshed server", "address", addr)
		return
	}

	server = &serverlist.Server{
		Addr: *addr,
		Age:  time.Now(),
	}

	err := serverList.AddServer(server)
	if err != nil {
		slog.Warn("Failed to add server", "address", addr, "error", err)
	} else {
		slog.Info("New server registered", "address", addr, "total", serverList.Size())
	}
}

func readString(reader *bytes.Reader) string {
	buf := make([]byte, 0)
	for {
		char, err := reader.ReadByte()
		if err != nil || char == 0 {
			break
		}
		buf = append(buf, char)
	}
	return string(buf)
}

func addServerInfo(addr *net.UDPAddr, reader *bytes.Reader) {
	server := serverList.GetServer(addr)
	if server == nil {
		slog.Warn("Received server info from unknown server", "address", addr)
		return
	}

	if server.KeySent == 0 {
		slog.Warn("Received server info from server without key", "address", addr)
		return
	}

	var discard uint32
	binary.Read(reader, binary.LittleEndian, &discard)
	var keySent uint32
	binary.Read(reader, binary.LittleEndian, &keySent)

	if server.KeySent != keySent {
		slog.Warn("Received server info with mismatched key", "address", addr)
		return
	}

	server.Verified = true
	server.Age = time.Now()

	server.Hostname = readString(reader)
	binary.Read(reader, binary.LittleEndian, &server.Numplayers)
	binary.Read(reader, binary.LittleEndian, &server.Maxplayers)
	server.Curmap = readString(reader)
	var numpwads byte
	binary.Read(reader, binary.LittleEndian, &numpwads)

	server.Pwads = make([]string, numpwads)
	for i := range server.Pwads {
		server.Pwads[i] = readString(reader)
	}

	binary.Read(reader, binary.LittleEndian, &server.Gametype)
	binary.Read(reader, binary.LittleEndian, &server.Skill)
	binary.Read(reader, binary.LittleEndian, &server.Teamplay)
	binary.Read(reader, binary.LittleEndian, &server.Ctfmode)

	server.Players = make([]serverlist.Player, server.Numplayers)
	for i := range server.Players {
		server.Players[i] = serverlist.Player{}
		server.Players[i].Name = readString(reader)
		binary.Read(reader, binary.LittleEndian, &server.Players[i].Frags)
		binary.Read(reader, binary.LittleEndian, &server.Players[i].Ping)
		binary.Read(reader, binary.LittleEndian, &server.Players[i].Team)
	}
}

func sendServerInfo(addr *net.UDPAddr, conn *net.UDPConn) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, uint32(LAUNCHER_CHALLENGE))
	var verified_count int16
	for server := range serverList.Servers() {
		if server.Verified {
			verified_count++
		}
	}
	binary.Write(buf, binary.LittleEndian, verified_count)

	for server := range serverList.Servers() {
		if !server.Verified {
			continue
		}

		ip := server.Addr.IP.To4()
		port := uint16(server.Addr.Port)
		for _, octet := range ip {
			binary.Write(buf, binary.LittleEndian, octet)
		}
		binary.Write(buf, binary.LittleEndian, port)
	}

	conn.WriteToUDP(buf.Bytes(), addr)
}

func handlePacket(n int, addr *net.UDPAddr, conn *net.UDPConn, reader *bytes.Reader) {
	var challenge uint32
	err := binary.Read(reader, binary.LittleEndian, &challenge)
	if err != nil {
		slog.Error("Failed to read challenge", "error", err)
		return
	}
	slog.Debug("Received packet", "address", addr, "challenge", challenge, "packetlen", n)
	switch challenge {
	case 0: // the c++ and python code have this, not sure why
	case SERVER_CHALLENGE:
		// actually handle server challenge
		if reader.Len() == 2 {
			// full response with server info
			var port uint16
			err := binary.Read(reader, binary.LittleEndian, &port)
			if err != nil {
				slog.Warn("Failed to read server port", "error", err)
				return
			}
			slog.Info("Received server heartbeat", "address", addr, "port", port)
			addr.Port = int(port)
			addServer(addr)
		} else if reader.Len() > 2 {
			// new server pinging master
			slog.Info("Received server info", "address", addr)
			addServerInfo(addr, reader)
		} else {
			// invalid packet, give error or warning
			slog.Warn("Received invalid server challenge")
		}
	case LAUNCHER_CHALLENGE:
		// actually handle launcher challenge
		slog.Info("Launcher requested list", "address", addr)
		sendServerInfo(addr, conn)
	default:
		slog.Warn("Received unknown challenge", "challenge", challenge)
	}
}

func pingServers(conn *net.UDPConn) {
	for server := range serverList.Servers() {
		if server.Pinged && !server.Verified {
			continue
		}
		server.KeySent = rand.Uint32N(0x7fffffff)

		buf := new(bytes.Buffer)
		binary.Write(buf, binary.LittleEndian, uint32(LAUNCHER_CHALLENGE))
		binary.Write(buf, binary.LittleEndian, server.KeySent)

		conn.WriteToUDP(buf.Bytes(), &server.Addr)

		server.Pinged = true
	}
}

func cullServers() {
	to_cull := make([]*serverlist.Server, 0)
	for server := range serverList.Servers() {
		if server.Verified {
			if time.Since(server.Age) > MAX_SERVER_AGE {
				to_cull = append(to_cull, server)
				slog.Info("Server timed out", "address", server.Addr)
			}
		} else {
			if time.Since(server.Age) > MAX_UNVERIFIED_SERVER_AGE {
				to_cull = append(to_cull, server)
				slog.Info("Unverified server timed out", "address", server.Addr)
			}
		}
	}
	for _, server := range to_cull {
		serverList.RemoveServer(server)
	}
}

func main() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: MASTER_PORT})
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to listen on port %d", MASTER_PORT), "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	slog.Info("Odamex Master Started.")

	buf := make([]byte, MAX_UDP_PACKET_SIZE)
	serverList = serverlist.New()

	next_ping := time.Now().Add(time.Second * 5)

	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			slog.Error("Failed to read from UDP", "error", err)
			continue
		}
		handlePacket(n, addr, conn, bytes.NewReader(buf[:n]))
		cullServers()
		if time.Now().After(next_ping) {
			pingServers(conn)
			next_ping = time.Now().Add(time.Second * 5)
		}
	}
}

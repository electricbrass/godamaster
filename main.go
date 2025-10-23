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
)

const MASTER_PORT = 15000

const MAX_SERVERS = 1024
const MAX_SERVERS_PER_IP = 64

// const MAX_SERVER_AGE =
// const MAX_UNVERIFIED_SERVER_AGE =

const SERVER_CHALLENGE = 5560020
const LAUNCHER_CHALLENGE = 777123

const MAX_UDP_PACKET_SIZE = 1400

type player struct {
	name  string
	frags int16
	ping  int32
	team  byte
}

type server struct {
	addr net.UDPAddr
	age  time.Time

	// server data
	hostname   string
	numplayers byte
	maxplayers byte
	curmap     string
	pwads      []string
	gametype   byte
	skill      byte
	teamplay   byte
	ctfmode    byte
	players    []player

	key_sent uint32
	verified bool
	pinged   bool
}

var servers map[string]*server
var addresses map[string]map[int]*server

func addServer(addr *net.UDPAddr) {
	strAddr := addr.String()
	strIP := addr.IP.String()
	srv := servers[strAddr]

	// server already exists?
	if srv != nil {
		srv.age = time.Now()
		srv.pinged = false
		slog.Debug("Refreshed server", "address", addr)
		return
	}

	srv = &server{
		addr: *addr,
		age:  time.Now(),
	}

	if servers == nil {
		servers = make(map[string]*server)
	}
	servers[strAddr] = srv

	if addresses == nil {
		addresses = make(map[string]map[int]*server)
	}
	if addresses[strIP] == nil {
		addresses[strIP] = make(map[int]*server)
	}
	addresses[strIP][addr.Port] = srv
}

func readString(reader *bytes.Reader) string {
	buf := make([]byte, 0)
	var char byte
	var err error
	for char = 7; char != 0; char, err = reader.ReadByte() {
		if err != nil {
			//
		}
		buf = append(buf, char)
	}
	return string(buf)
}

func addServerInfo(addr *net.UDPAddr, reader *bytes.Reader) {
	srv := servers[addr.String()]
	if srv == nil {
		slog.Warn("Received server info from unknown server", "address", addr)
		return
	}

	if srv.key_sent == 0 {
		slog.Warn("Received server info from server without key", "address", addr)
		return
	}

	var discard uint32
	binary.Read(reader, binary.LittleEndian, &discard)
	var key_sent uint32
	binary.Read(reader, binary.LittleEndian, &key_sent)

	if srv.key_sent != key_sent {
		slog.Warn("Received server info with mismatched key", "address", addr)
		return
	}

	srv.verified = true
	srv.age = time.Now()

	srv.hostname = readString(reader)
	binary.Read(reader, binary.LittleEndian, &srv.numplayers)
	binary.Read(reader, binary.LittleEndian, &srv.maxplayers)
	srv.curmap = readString(reader)
	var numpwads byte
	binary.Read(reader, binary.LittleEndian, &numpwads)

	srv.pwads = make([]string, numpwads)
	for i := range srv.pwads {
		srv.pwads[i] = readString(reader)
	}

	binary.Read(reader, binary.LittleEndian, &srv.gametype)
	binary.Read(reader, binary.LittleEndian, &srv.skill)
	binary.Read(reader, binary.LittleEndian, &srv.teamplay)
	binary.Read(reader, binary.LittleEndian, &srv.ctfmode)

	srv.players = make([]player, srv.numplayers)
	for i := range srv.players {
		srv.players[i] = player{}
		srv.players[i].name = readString(reader)
		binary.Read(reader, binary.LittleEndian, &srv.players[i].frags)
		binary.Read(reader, binary.LittleEndian, &srv.players[i].ping)
		binary.Read(reader, binary.LittleEndian, &srv.players[i].team)
	}
}

func sendServerInfo(addr *net.UDPAddr, conn *net.UDPConn) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, uint32(LAUNCHER_CHALLENGE))
	var verified_count int16
	for _, server := range servers {
		if server.verified {
			verified_count++
		}
	}
	binary.Write(buf, binary.LittleEndian, verified_count)

	for _, server := range servers {
		if !server.verified {
			continue
		}

		ip := server.addr.IP.To4()
		port := uint16(server.addr.Port)
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
		// do something with error
	}
	switch challenge {
	case 0: // the c++ and python code have this, not sure why
	case SERVER_CHALLENGE:
		// actually handle server challenge
		if reader.Len() == 2 {
			// full response with server info
			var port uint16
			err := binary.Read(reader, binary.LittleEndian, &port)
			if err != nil {
				// do something with error
			}
			slog.Info("Received server challenge", "port", port)
			addr.Port = int(port)
			addServer(addr)
		} else if reader.Len() > 2 {
			// new server pinging master
			slog.Info("hey new server info")
			addServerInfo(addr, reader)
		} else {
			// invalid packet, give error or warning
			slog.Warn("Received invalid server challenge.")
		}
	case LAUNCHER_CHALLENGE:
		// actually handle launcher challenge
		slog.Info("Hey a launcher challenge.")
		sendServerInfo(addr, conn)
	default:
		slog.Warn("Received unknown challenge", "challenge", challenge)
	}
}

func pingServers(conn *net.UDPConn) {
	for _, server := range servers {
		if server.pinged && !server.verified {
			continue
		}
		server.key_sent = rand.Uint32N(0x7fffffff)

		buf := new(bytes.Buffer)
		binary.Write(buf, binary.LittleEndian, uint32(LAUNCHER_CHALLENGE))
		binary.Write(buf, binary.LittleEndian, server.key_sent)

		conn.WriteToUDP(buf.Bytes(), &server.addr)

		server.pinged = true
	}
}

// func cullServers() {
// 	to_cull := make([]string, 0);
// }

func main() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: MASTER_PORT})
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to listen on port %d", MASTER_PORT), "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	slog.Info("Odamex Master Started.")

	buf := make([]byte, MAX_UDP_PACKET_SIZE)

	next_ping := time.Now().Add(time.Second * 5)

	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			slog.Error("Failed to read from UDP", "error", err)
			continue
		}
		handlePacket(n, addr, conn, bytes.NewReader(buf[:n]))
		if time.Now().After(next_ping) {
			pingServers(conn)
			next_ping = time.Now().Add(time.Second * 5)
		}
	}
}

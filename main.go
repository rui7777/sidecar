package main

import (
	coresdk "agones.dev/agones/pkg/sdk"
	"agones.dev/agones/pkg/util/runtime"
	sdk "agones.dev/agones/sdks/go"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	logger = runtime.NewLoggerWithSource("main")
)

type clientRequest struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    string `json:"port"`
	Message string `json:"message"`
}

type clientResponse struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    string `json:"port"`
}

func main() {
	port := flag.String("port", "8000", "The port to listen to TCP requests")
	passthrough := flag.Bool("passthrough", false, "Get listening port from the SDK, rather than use the 'port' value")

	flag.Parse()
	if ep := os.Getenv("PORT"); ep != "" {
		port = &ep
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		listener(w, r, port, passthrough)
	})

	logger.Info("Starting HTTP server on port ", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		logger.WithError(err).Fatal("HTTP server failed to run")
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, "Healthy")
	if err != nil {
		logger.WithError(err).Fatal("Error writing string Healthy from /health")
	}
}

func listener(w http.ResponseWriter, r *http.Request, port *string, passthrough *bool) {
	var cRequest clientRequest
	json.NewDecoder(r.Body).Decode(&cRequest)

	stop := make(chan struct{})

	s, err := sdk.NewSDK()
	if err != nil {
		log.Fatalf("Could not connect to sdk: %v", err)
	}
	if *passthrough {
		var gs *coresdk.GameServer
		gs, err = s.GameServer()
		if err != nil {
			log.Fatalf("Could not get gameserver port details: %s", err)
		}

		p := strconv.FormatInt(int64(gs.Status.Ports[0].Port), 10)
		port = &p
	}

	_, err = handleResponse(cRequest.Message, s, stop)

	cResponse := clientResponse{
		Name: cRequest.Name,
		Host: cRequest.Host,
		Port: cRequest.Port,
	}

	res, err := json.Marshal(cResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

func handleResponse(txt string, s *sdk.SDK, stop chan struct{}) (response string, responseError error) {
	parts := strings.Split(strings.TrimSpace(txt), " ")
	response = txt
	responseError = nil

	switch parts[0] {
	// shuts down the gameserver
	case "EXIT":
		// handle elsewhere, as we respond before exiting
		return

	// turns off the health pings
	case "UNHEALTHY":
		close(stop)

	case "GAMESERVER":
		response = gameServerName(s)

	case "READY":
		ready(s)

	case "ALLOCATE":
		allocate(s)

	case "RESERVE":
		if len(parts) != 2 {
			response = "Invalid RESERVE, should have 1 argument"
			responseError = fmt.Errorf("Invalid RESERVE, should have 1 argument")
		}
		if dur, err := time.ParseDuration(parts[1]); err != nil {
			response = fmt.Sprintf("%s\n", err)
			responseError = err
		} else {
			reserve(s, dur)
		}

	case "WATCH":
		watchGameServerEvents(s)

	case "LABEL":
		switch len(parts) {
		case 1:
			// legacy format
			setLabel(s, "timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		case 3:
			setLabel(s, parts[1], parts[2])
		default:
			response = "Invalid LABEL command, must use zero or 2 arguments"
			responseError = fmt.Errorf("Invalid LABEL command, must use zero or 2 arguments")
		}

	case "CRASH":
		log.Print("Crashing.")
		os.Exit(1)
		return "", nil

	case "ANNOTATION":
		switch len(parts) {
		case 1:
			// legacy format
			setAnnotation(s, "timestamp", time.Now().UTC().String())
		case 3:
			setAnnotation(s, parts[1], parts[2])
		default:
			response = "Invalid ANNOTATION command, must use zero or 2 arguments"
			responseError = fmt.Errorf("Invalid ANNOTATION command, must use zero or 2 arguments")
		}

	case "PLAYER_CAPACITY":
		switch len(parts) {
		case 1:
			response = getPlayerCapacity(s)
		case 2:
			if cap, err := strconv.Atoi(parts[1]); err != nil {
				response = fmt.Sprintf("%s", err)
				responseError = err
			} else {
				setPlayerCapacity(s, int64(cap))
			}
		default:
			response = "Invalid PLAYER_CAPACITY, should have 0 or 1 arguments"
			responseError = fmt.Errorf("Invalid PLAYER_CAPACITY, should have 0 or 1 arguments")
		}

	case "PLAYER_CONNECT":
		if len(parts) < 2 {
			response = "Invalid PLAYER_CONNECT, should have 1 arguments"
			responseError = fmt.Errorf("Invalid PLAYER_CONNECT, should have 1 arguments")
			return
		}
		playerConnect(s, parts[1])

	case "PLAYER_DISCONNECT":
		if len(parts) < 2 {
			response = "Invalid PLAYER_DISCONNECT, should have 1 arguments"
			responseError = fmt.Errorf("Invalid PLAYER_DISCONNECT, should have 1 arguments")
			return
		}
		playerDisconnect(s, parts[1])

	case "PLAYER_CONNECTED":
		if len(parts) < 2 {
			response = "Invalid PLAYER_CONNECTED, should have 1 arguments"
			responseError = fmt.Errorf("Invalid PLAYER_CONNECTED, should have 1 arguments")
			return
		}
		response = playerIsConnected(s, parts[1])

	case "GET_PLAYERS":
		response = getConnectedPlayers(s)

	case "PLAYER_COUNT":
		response = getPlayerCount(s)
	}

	return
}

// ready attempts to mark this gameserver as ready
func ready(s *sdk.SDK) {
	err := s.Ready()
	if err != nil {
		log.Fatalf("Could not send ready message")
	}
}

// allocate attempts to allocate this gameserver
func allocate(s *sdk.SDK) {
	err := s.Allocate()
	if err != nil {
		log.Fatalf("could not allocate gameserver: %v", err)
	}
}

// reserve for 10 seconds
func reserve(s *sdk.SDK, duration time.Duration) {
	if err := s.Reserve(duration); err != nil {
		log.Fatalf("could not reserve gameserver: %v", err)
	}
}

// readPacket reads a string from the connection
func readPacket(conn net.PacketConn, b []byte) (net.Addr, string) {
	n, sender, err := conn.ReadFrom(b)
	if err != nil {
		log.Fatalf("Could not read from udp stream: %v", err)
	}
	txt := strings.TrimSpace(string(b[:n]))
	log.Printf("Received packet from %v: %v", sender.String(), txt)
	return sender, txt
}

// exit shutdowns the server
func exit(s *sdk.SDK) {
	log.Printf("Received EXIT command. Exiting.")
	// This tells Agones to shutdown this Game Server
	shutdownErr := s.Shutdown()
	if shutdownErr != nil {
		log.Printf("Could not shutdown")
	}
	os.Exit(0)
}

// gameServerName returns the GameServer name
func gameServerName(s *sdk.SDK) string {
	var gs *coresdk.GameServer
	gs, err := s.GameServer()
	if err != nil {
		log.Fatalf("Could not retrieve GameServer: %v", err)
	}
	var j []byte
	j, err = json.Marshal(gs)
	if err != nil {
		log.Fatalf("error mashalling GameServer to JSON: %v", err)
	}
	log.Printf("GameServer: %s \n", string(j))
	return "NAME: " + gs.ObjectMeta.Name + "\n"
}

// watchGameServerEvents creates a callback to log when
// gameserver events occur
func watchGameServerEvents(s *sdk.SDK) {
	err := s.WatchGameServer(func(gs *coresdk.GameServer) {
		j, err := json.Marshal(gs)
		if err != nil {
			log.Fatalf("error mashalling GameServer to JSON: %v", err)
		}
		log.Printf("GameServer Event: %s \n", string(j))
	})
	if err != nil {
		log.Fatalf("Could not watch Game Server events, %v", err)
	}
}

// setAnnotation sets a given annotation
func setAnnotation(s *sdk.SDK, key, value string) {
	log.Printf("Setting annotation %v=%v", key, value)
	err := s.SetAnnotation(key, value)
	if err != nil {
		log.Fatalf("could not set annotation: %v", err)
	}
}

// setLabel sets a given label
func setLabel(s *sdk.SDK, key, value string) {
	log.Printf("Setting label %v=%v", key, value)
	// label values can only be alpha, - and .
	err := s.SetLabel(key, value)
	if err != nil {
		log.Fatalf("could not set label: %v", err)
	}
}

// setPlayerCapacity sets the player capacity to the given value
func setPlayerCapacity(s *sdk.SDK, capacity int64) {
	log.Printf("Setting Player Capacity to %d", capacity)
	if err := s.Alpha().SetPlayerCapacity(capacity); err != nil {
		log.Fatalf("could not set capacity: %v", err)
	}
}

// getPlayerCapacity returns the current player capacity as a string
func getPlayerCapacity(s *sdk.SDK) string {
	log.Print("Getting Player Capacity")
	capacity, err := s.Alpha().GetPlayerCapacity()
	if err != nil {
		log.Fatalf("could not get capacity: %v", err)
	}
	return strconv.FormatInt(capacity, 10) + "\n"
}

// playerConnect connects a given player
func playerConnect(s *sdk.SDK, id string) {
	log.Printf("Connecting Player: %s", id)
	if _, err := s.Alpha().PlayerConnect(id); err != nil {
		log.Fatalf("could not connect player: %v", err)
	}
}

// playerDisconnect disconnects a given player
func playerDisconnect(s *sdk.SDK, id string) {
	log.Printf("Disconnecting Player: %s", id)
	if _, err := s.Alpha().PlayerDisconnect(id); err != nil {
		log.Fatalf("could not disconnect player: %v", err)
	}
}

// playerIsConnected returns a bool as a string if a player is connected
func playerIsConnected(s *sdk.SDK, id string) string {
	log.Printf("Checking if player %s is connected", id)

	connected, err := s.Alpha().IsPlayerConnected(id)
	if err != nil {
		log.Fatalf("could not retrieve if player is connected: %v", err)
	}

	return strconv.FormatBool(connected) + "\n"
}

// getConnectedPlayers returns a comma delimeted list of connected players
func getConnectedPlayers(s *sdk.SDK) string {
	log.Print("Retrieving connected player list")
	list, err := s.Alpha().GetConnectedPlayers()
	if err != nil {
		log.Fatalf("could not retrieve connected players: %s", err)
	}

	return strings.Join(list, ",") + "\n"
}

// getPlayerCount returns the count of connected players as a string
func getPlayerCount(s *sdk.SDK) string {
	log.Print("Retrieving connected player count")
	count, err := s.Alpha().GetPlayerCount()
	if err != nil {
		log.Fatalf("could not retrieve player count: %s", err)
	}
	return strconv.FormatInt(count, 10) + "\n"
}
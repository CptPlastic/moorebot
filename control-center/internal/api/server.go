package api

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cptplastic/moorebot/control-center/internal/botcfg"
	"github.com/cptplastic/moorebot/control-center/internal/ros"
	"github.com/gorilla/websocket"
)

type Server struct {
	ROS    *ros.Client
	BotSSH *botcfg.Client
	Wifi   *botcfg.WifiCache
	UI     *uiStore
	Static http.FileSystem

	cameraMu   sync.Mutex
	pilotMu    sync.Mutex
	pilotSeq   atomic.Uint64
	pilotOwner uint64
}

type driveMsg struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	AZ   float64 `json:"az"`
	Spd  float64 `json:"spd"`
	Cmd  string  `json:"cmd"`
	T    int64   `json:"t"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser client
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.UI == nil {
		s.UI = NewUIStore()
	}
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/ui", s.handleUI)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/ws", s.handleWS)
	if s.Static != nil {
		mux.Handle("/", http.FileServer(s.Static))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	width, height := s.ROS.Resolution()
	batt := s.ROS.Battery()
	night := s.ROS.Night()
	nightMode, cameraLight := s.ROS.VideoMode()
	modeLabel := map[int32]string{0: "color", 1: "ir", 2: "auto"}[nightMode]
	nightOut := map[string]any{
		"mode": nightMode, "modeLabel": modeLabel, "cameraLight": cameraLight,
		"isNight": nightMode == ros.NightModeIR || nightMode == ros.NightModeAuto,
	}
	if night != nil {
		nightOut["isNight"] = night.IsNight != 0
		nightOut["brightness"] = night.Brightness
	}
	var wifi any
	if s.Wifi != nil {
		wifi = s.Wifi.Get()
	}
	ui := s.UI.Get()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"battery":    batt,
		"codec":      "h264+aac",
		"frames":     s.ROS.FrameCount(),
		"audio":      s.ROS.AudioCount(),
		"frameAgeMs": s.ROS.FrameAgeMs(),
		"width":      width,
		"height":     height,
		"tracked":    s.ROS.Tracked(),
		"dock":       s.ROS.Dock(),
		"night":      nightOut,
		"wifi":       wifi,
		"ui":         ui,
	})
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"ok": true, "ui": s.UI.Get()})
	case http.MethodPost, http.MethodPut:
		var body struct {
			Reticle *bool    `json:"reticle"`
			Hud     *bool    `json:"hud"`
			Mission *Mission `json:"mission"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		patch := UIState{}
		if body.Reticle != nil {
			patch.Reticle = *body.Reticle
		}
		if body.Hud != nil {
			patch.Hud = *body.Hud
		}
		if body.Mission != nil {
			patch.Mission = *body.Mission
		}
		ui := s.UI.Patch(patch, body.Reticle != nil, body.Hud != nil, body.Mission != nil)
		writeJSON(w, 200, map[string]any{"ok": true, "ui": ui})
	default:
		writeJSON(w, 405, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"logs": s.ROS.Logs()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m, err := s.BotSSH.ReadMotion()
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.ROS.SetTracked(m.Track == 1)
		writeJSON(w, 200, map[string]any{
			"ok": true, "track": m.Track == 1, "mecanum": m.Track == 0, "vio": m.VIO == 1, "motion": m,
		})
	case http.MethodPost:
		var body struct {
			Track *bool `json:"track"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if body.Track == nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "track required"})
			return
		}
		m, err := s.BotSSH.SetTrack(*body.Track)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.ROS.SetTracked(m.Track == 1)
		_ = s.ROS.Stop()
		writeJSON(w, 200, map[string]any{
			"ok": true, "track": m.Track == 1, "mecanum": m.Track == 0, "motion": m,
			"note": "motor_node bounced to reload kinematics",
		})
	default:
		writeJSON(w, 405, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) setNightMode(mode int32) error {
	labels := map[int32]string{0: "color/day", 1: "IR on", 2: "IR auto"}
	s.ROS.Log("info", "camera → "+labels[mode])
	if err := s.ROS.SetNightMode(mode); err != nil {
		return err
	}
	s.ROS.RefreshNight()
	return nil
}

func (s *Server) setCameraLight(level int32) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	s.ROS.Log("info", fmt.Sprintf("IR light → %d", level))
	if err := s.ROS.SetCameraLight(level); err != nil {
		return err
	}
	s.ROS.RefreshNight()
	return nil
}

func (s *Server) setVideoMode(mode, light int32) error {
	labels := map[int32]string{0: "color/day", 1: "IR on", 2: "IR auto"}
	s.ROS.Log("info", fmt.Sprintf("camera → %s, light=%d", labels[mode], light))
	if err := s.ROS.SetVideoMode(mode, light); err != nil {
		return err
	}
	s.ROS.RefreshNight()
	return nil
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok": false, "error": "method not allowed",
		})
		return
	}
	var body struct {
		Cmd string `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": err.Error(),
		})
		return
	}
	if isCameraAction(body.Cmd) {
		s.cameraMu.Lock()
		defer s.cameraMu.Unlock()
	}
	var err error
	switch body.Cmd {
	case "stop":
		err = s.ROS.ForceStop()
	case "ir_on":
		err = s.setVideoMode(ros.NightModeIR, 80)
	case "ir_off", "color":
		// LEDs off plus mechanical IR-cut in for stable color mode.
		err = s.setVideoMode(ros.NightModeColor, 0)
	case "ir_auto":
		err = s.setNightMode(ros.NightModeAuto)
	case "ir_light_on":
		// The illuminator is independent from the mechanical IR-cut filter.
		err = s.setCameraLight(100)
	case "ir_light_off":
		err = s.setCameraLight(0)
	case "light_up":
		_, light := s.ROS.VideoMode()
		err = s.setCameraLight(light + 10)
	case "light_down":
		_, light := s.ROS.VideoMode()
		err = s.setCameraLight(light - 10)
	case "refresh_night":
		s.ROS.RefreshNight()
	default:
		writeJSON(w, 400, map[string]any{"ok": false, "error": "unknown cmd"})
		return
	}
	if err != nil {
		s.ROS.Log("error", body.Cmd+": "+err.Error())
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "cmd": body.Cmd})
}

func isCameraAction(cmd string) bool {
	switch cmd {
	case "ir_on", "ir_off", "color", "ir_auto",
		"ir_light_on", "ir_light_off", "light_up", "light_down":
		return true
	default:
		return false
	}
}

func (s *Server) claimPilot(id uint64) bool {
	s.pilotMu.Lock()
	defer s.pilotMu.Unlock()
	if s.pilotOwner == 0 || s.pilotOwner == id {
		s.pilotOwner = id
		return true
	}
	return false
}

func (s *Server) releasePilot(id uint64) bool {
	s.pilotMu.Lock()
	defer s.pilotMu.Unlock()
	if id != 0 && s.pilotOwner == id {
		s.pilotOwner = 0
		return true
	}
	return false
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	viewOnly := r.URL.Query().Get("view") == "1"
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var pilotID uint64
	if !viewOnly {
		pilotID = s.pilotSeq.Add(1)
	}
	s.ROS.Log("info", map[bool]string{true: "view client connected", false: "pilot client connected"}[viewOnly])

	var writeMu sync.Mutex
	var sendingVid atomic.Bool
	var sendingAud atomic.Bool

	writeBin := func(buf []byte) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(400 * time.Millisecond))
		_ = conn.WriteMessage(websocket.BinaryMessage, buf)
	}
	writeJSON := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(400 * time.Millisecond))
		_ = conn.WriteMessage(websocket.TextMessage, b)
	}

	// Seed recent logs
	for _, line := range s.ROS.Logs() {
		writeJSON(map[string]any{"type": "log", "log": line})
	}

	unsubLog := s.ROS.OnLog(func(line ros.LogLine) {
		writeJSON(map[string]any{"type": "log", "log": line})
	})
	defer unsubLog()

	unsubVid := s.ROS.OnH264(func(pkt ros.VideoPacket) {
		if !sendingVid.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer sendingVid.Store(false)
			buf := make([]byte, 6+len(pkt.Data))
			buf[0] = 1 // video
			if pkt.KeyFrame {
				buf[1] = 1
			}
			w, h := pkt.Width, pkt.Height
			if w <= 0 || h <= 0 {
				w, h = s.ROS.Resolution()
			}
			binary.BigEndian.PutUint16(buf[2:4], uint16(w))
			binary.BigEndian.PutUint16(buf[4:6], uint16(h))
			copy(buf[6:], pkt.Data)
			writeBin(buf)
		}()
	})
	defer unsubVid()

	unsubAud := s.ROS.OnAAC(func(pkt ros.AudioPacket) {
		if !sendingAud.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer sendingAud.Store(false)
			buf := make([]byte, 1+len(pkt.Data))
			buf[0] = 2 // audio
			copy(buf[1:], pkt.Data)
			writeBin(buf)
		}()
	})
	defer unsubAud()

	if viewOnly {
		// Media-only socket — just keep alive until disconnect.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				s.ROS.Log("info", "view client disconnected")
				return
			}
		}
	}

	stopDrive := make(chan struct{})
	defer close(stopDrive)
	var lastDriveUnix atomic.Int64
	var driving atomic.Bool
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stopDrive:
				if s.releasePilot(pilotID) {
					_ = s.ROS.Stop()
				}
				return
			case <-t.C:
				last := lastDriveUnix.Load()
				if driving.Load() && last > 0 &&
					time.Since(time.Unix(0, last)) > 400*time.Millisecond &&
					driving.CompareAndSwap(true, false) {
					if s.releasePilot(pilotID) {
						_ = s.ROS.Stop()
					}
				}
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if s.releasePilot(pilotID) {
				_ = s.ROS.Stop()
			}
			s.ROS.Log("info", "pilot client disconnected")
			return
		}
		var msg driveMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type == "ping" {
			writeJSON(map[string]any{"type": "pong", "t": msg.T})
			continue
		}
		if msg.Type == "action" {
			// UI actions are accepted only by /api/action. Older cached pages sent
			// both WS and HTTP, which physically toggled the IR-cut filter twice.
			continue
		}
		spd := msg.Spd
		if spd <= 0 {
			spd = 0.25
		}
		if spd > 1 {
			spd = 1
		}
		x, y, az := msg.X*0.2, msg.Y*spd, msg.AZ*-1.8
		if x == 0 && y == 0 && az == 0 {
			if driving.Swap(false) {
				if s.releasePilot(pilotID) {
					_ = s.ROS.Drive(0, 0, 0)
				}
			}
			continue
		}
		if !s.claimPilot(pilotID) {
			continue
		}
		driving.Store(true)
		lastDriveUnix.Store(time.Now().UnixNano())
		_ = s.ROS.Drive(x, y, az)
	}
}

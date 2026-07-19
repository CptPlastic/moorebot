package ros

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/goroslib/v2"
	"github.com/bluenviron/goroslib/v2/pkg/msgs/geometry_msgs"
	"github.com/bluenviron/goroslib/v2/pkg/msgs/std_msgs"
)

// VideoPacket is one Annex-B H.264 access unit from /CoreNode/h264.
type VideoPacket struct {
	Data      []byte
	KeyFrame  bool
	Width     int
	Height    int
	Timestamp uint64
}

// AudioPacket is AAC from /CoreNode/aac (usually ADTS).
type AudioPacket struct {
	Data      []byte
	Timestamp uint64
	Par1      int32
	Par2      int32
}

// BatteryInfo is decoded from /SensorNode/simple_battery_status.
type BatteryInfo struct {
	Percent int     `json:"percent"`
	State   int     `json:"state"`
	Label   string  `json:"label"`
	Raw     []int32 `json:"raw,omitempty"`
}

// LogLine is a control-center event for the UI console.
type LogLine struct {
	TS    int64  `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// Client is a thin ROS1 bridge to the Scout.
type Client struct {
	node            *goroslib.Node
	cmdPub          *goroslib.Publisher
	cancelDetectPub *goroslib.Publisher

	cmdMu         sync.Mutex
	mu            sync.RWMutex
	battery       BatteryInfo
	dock          DockStatus
	night         *NightGetRes
	nightMode     int32 // image_night_mode: 0 color, 1 IR, 2 auto
	cameraLight   int32 // IR LED level 0..100
	width         int32
	height        int32
	frameSeq      uint64
	audioSeq      uint64
	lastFrameUnix int64 // unix nanos
	nightErr      atomic.Bool
	tracked       bool
	autonomy      bool
	dockSession   bool
	wasDriving    bool

	h264ID   uint64
	h264Subs map[uint64]func(VideoPacket)
	aacID    uint64
	aacSubs  map[uint64]func(AudioPacket)

	logs    []LogLine
	logSubs map[uint64]func(LogLine)
	logID   uint64

	backupSub *goroslib.Subscriber
}

type Config struct {
	MasterAddress string
	Host          string
	NodeName      string
}

func NewOfflineClient() *Client {
	c := &Client{
		tracked:  true,
		h264Subs: map[uint64]func(VideoPacket){},
		aacSubs:  map[uint64]func(AudioPacket){},
		logSubs:  map[uint64]func(LogLine){},
		logs:     make([]LogLine, 0, 200),
	}
	c.Log("warn", "Scout offline — waiting for ROS")
	return c
}

func Connect(cfg Config) (*Client, error) {
	if cfg.NodeName == "" {
		cfg.NodeName = "moorebot_control_center"
	}
	n, err := goroslib.NewNode(goroslib.NodeConf{
		Name:          cfg.NodeName,
		MasterAddress: cfg.MasterAddress,
		Host:          cfg.Host,
	})
	if err != nil {
		return nil, fmt.Errorf("ros node: %w", err)
	}

	c := &Client{
		node:     n,
		tracked:  true,
		h264Subs: map[uint64]func(VideoPacket){},
		aacSubs:  map[uint64]func(AudioPacket){},
		logSubs:  map[uint64]func(LogLine){},
		logs:     make([]LogLine, 0, 200),
	}

	pub, err := goroslib.NewPublisher(goroslib.PublisherConf{
		Node:  n,
		Topic: "/cmd_vel",
		Msg:   &geometry_msgs.Twist{},
	})
	if err != nil {
		n.Close()
		return nil, fmt.Errorf("cmd_vel publisher: %w", err)
	}
	c.cmdPub = pub

	cancelPub, err := goroslib.NewPublisher(goroslib.PublisherConf{
		Node:  n,
		Topic: "/AppNode/cancel_detect",
		Msg:   &std_msgs.Int32{},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("cancel_detect publisher: %w", err)
	}
	c.cancelDetectPub = cancelPub

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/CoreNode/h264",
		QueueSize: 1,
		Callback: func(msg *Frame) {
			if msg == nil || len(msg.Data) == 0 {
				return
			}
			data := append([]byte(nil), msg.Data...)
			w, h := int(msg.Par1), int(msg.Par2)
			key := msg.Par3 == 1 || isKeyFrame(data)
			if w > 0 {
				atomic.StoreInt32(&c.width, int32(w))
			}
			if h > 0 {
				atomic.StoreInt32(&c.height, int32(h))
			}
			pkt := VideoPacket{Data: data, KeyFrame: key, Width: w, Height: h, Timestamp: msg.Stamp}
			c.mu.RLock()
			subs := make([]func(VideoPacket), 0, len(c.h264Subs))
			for _, cb := range c.h264Subs {
				subs = append(subs, cb)
			}
			c.mu.RUnlock()
			for _, cb := range subs {
				cb(pkt)
			}
			atomic.AddUint64(&c.frameSeq, 1)
			atomic.StoreInt64(&c.lastFrameUnix, time.Now().UnixNano())
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("h264 subscriber: %w", err)
	}

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/CoreNode/aac",
		QueueSize: 8,
		Callback: func(msg *Frame) {
			if msg == nil || len(msg.Data) == 0 {
				return
			}
			pkt := AudioPacket{
				Data:      append([]byte(nil), msg.Data...),
				Timestamp: msg.Stamp,
				Par1:      msg.Par1,
				Par2:      msg.Par2,
			}
			c.mu.RLock()
			subs := make([]func(AudioPacket), 0, len(c.aacSubs))
			for _, cb := range c.aacSubs {
				subs = append(subs, cb)
			}
			c.mu.RUnlock()
			for _, cb := range subs {
				cb(pkt)
			}
			atomic.AddUint64(&c.audioSeq, 1)
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("aac subscriber: %w", err)
	}

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/SensorNode/simple_battery_status",
		QueueSize: 1,
		Callback: func(msg *Status) {
			if msg == nil || len(msg.Status) < 2 {
				return
			}
			info := BatteryInfo{
				State:   int(msg.Status[0]),
				Percent: int(msg.Status[1]),
				Raw:     append([]int32(nil), msg.Status...),
			}
			switch info.State {
			case BatteryCharging:
				info.Label = "charging"
			case BatteryUncharge:
				info.Label = "discharging"
			case BatteryFull:
				info.Label = "full"
			default:
				info.Label = "unknown"
			}
			c.mu.Lock()
			prev := c.battery.Percent
			c.battery = info
			c.mu.Unlock()
			if info.Percent != prev && (info.Percent%10 == 0 || info.Percent <= 15) {
				c.Log("info", fmt.Sprintf("battery %d%% (%s)", info.Percent, info.Label))
			}
			if info.State == BatteryCharging || info.State == BatteryFull {
				c.clearAutonomyIfDocked()
			}
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("battery subscriber: %w", err)
	}

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/CoreNode/going_home_status",
		QueueSize: 5,
		Callback: func(msg *std_msgs.Int32) {
			if msg == nil {
				return
			}
			c.setDockFromCode(int(msg.Data))
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("going_home_status subscriber: %w", err)
	}

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/CoreNode/backing_up_status",
		QueueSize: 5,
		Callback: func(msg *Status) {
			if msg == nil || len(msg.Status) < 1 {
				return
			}
			c.setDockFromCode(int(msg.Status[0]))
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("backing_up_status subscriber: %w", err)
	}

	_, err = goroslib.NewSubscriber(goroslib.SubscriberConf{
		Node:      n,
		Topic:     "/ParamNode/video/parameter_updates",
		QueueSize: 1,
		Callback: func(msg *DynConfig) {
			if msg == nil {
				return
			}
			c.mu.Lock()
			oldMode, oldLight := c.nightMode, c.cameraLight
			for _, p := range msg.Ints {
				switch p.Name {
				case "image_night_mode":
					c.nightMode = p.Value
				case "cameraLight":
					c.cameraLight = p.Value
				}
			}
			newMode, newLight := c.nightMode, c.cameraLight
			c.mu.Unlock()
			if oldMode != newMode || oldLight != newLight {
				c.Log("info", fmt.Sprintf(
					"camera state → mode=%d light=%d", newMode, newLight,
				))
			}
		},
	})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("video parameter subscriber: %w", err)
	}

	c.Log("info", "ROS bridge connected")
	return c, nil
}

func (c *Client) Close() {
	c.endBackupSub()
	if c.cancelDetectPub != nil {
		c.cancelDetectPub.Close()
	}
	if c.cmdPub != nil {
		c.cmdPub.Close()
	}
	if c.node != nil {
		c.node.Close()
	}
}

func (c *Client) Log(level, msg string) {
	line := LogLine{TS: time.Now().UnixMilli(), Level: level, Msg: msg}
	c.mu.Lock()
	c.logs = append(c.logs, line)
	if len(c.logs) > 200 {
		c.logs = c.logs[len(c.logs)-200:]
	}
	subs := make([]func(LogLine), 0, len(c.logSubs))
	for _, cb := range c.logSubs {
		subs = append(subs, cb)
	}
	c.mu.Unlock()
	for _, cb := range subs {
		cb(line)
	}
}

func (c *Client) Logs() []LogLine {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]LogLine, len(c.logs))
	copy(out, c.logs)
	return out
}

func (c *Client) Connected() bool {
	return c.node != nil
}

func (c *Client) OnH264(cb func(VideoPacket)) (cancel func()) {
	c.mu.Lock()
	c.h264ID++
	id := c.h264ID
	c.h264Subs[id] = cb
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.h264Subs, id)
		c.mu.Unlock()
	}
}

func (c *Client) OnAAC(cb func(AudioPacket)) (cancel func()) {
	c.mu.Lock()
	c.aacID++
	id := c.aacID
	c.aacSubs[id] = cb
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.aacSubs, id)
		c.mu.Unlock()
	}
}

func (c *Client) OnLog(cb func(LogLine)) (cancel func()) {
	c.mu.Lock()
	c.logID++
	id := c.logID
	c.logSubs[id] = cb
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.logSubs, id)
		c.mu.Unlock()
	}
}

func (c *Client) Battery() BatteryInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.battery
}

func (c *Client) Dock() DockStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dock
}

func (c *Client) Night() *NightGetRes {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.night == nil {
		return nil
	}
	n := *c.night
	return &n
}

func (c *Client) Autonomy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.autonomy
}

func (c *Client) SetTracked(tracked bool) {
	c.mu.Lock()
	c.tracked = tracked
	c.mu.Unlock()
	c.Log("info", fmt.Sprintf("drive base → %s", map[bool]string{true: "tracked", false: "mecanum"}[tracked]))
}

func (c *Client) Tracked() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tracked
}

func (c *Client) Resolution() (int, int) {
	return int(atomic.LoadInt32(&c.width)), int(atomic.LoadInt32(&c.height))
}

func (c *Client) FrameCount() uint64 { return atomic.LoadUint64(&c.frameSeq) }
func (c *Client) AudioCount() uint64 { return atomic.LoadUint64(&c.audioSeq) }

func (c *Client) FrameAgeMs() int64 {
	ns := atomic.LoadInt64(&c.lastFrameUnix)
	if ns == 0 {
		return -1
	}
	return (time.Now().UnixNano() - ns) / int64(time.Millisecond)
}

func (c *Client) setDockFromCode(code int) {
	ds := DockStatus{Code: code}
	switch code {
	case DockDetect:
		ds.Label, ds.Phase = "searching for dock", "searching"
	case DockAlign:
		ds.Label, ds.Phase = "aligning", "docking"
	case DockBackup:
		ds.Label, ds.Phase = "backing onto dock", "docking"
	case DockSuccess:
		ds.Label, ds.Phase = "docked", "done"
	case DockFail:
		ds.Label, ds.Phase = "dock failed", "fail"
	case DockCancel:
		ds.Label, ds.Phase = "cancelled", "idle"
	case DockInactive:
		ds.Label, ds.Phase = "idle", "idle"
	default:
		ds.Label, ds.Phase = fmt.Sprintf("status %d", code), "idle"
	}

	c.mu.Lock()
	session := c.dockSession
	if !session {
		if ds.Phase != "idle" {
			c.dock = DockStatus{Code: code, Label: ds.Label + " (bg)", Phase: "idle"}
		}
		c.autonomy = false
		c.mu.Unlock()
		return
	}
	c.dock = ds
	needEnd := false
	switch ds.Phase {
	case "searching", "docking":
		c.autonomy = true
	case "done", "fail":
		c.autonomy = false
		c.dockSession = false
		needEnd = true
	}
	c.mu.Unlock()
	c.Log("info", "dock: "+ds.Label)
	if needEnd {
		c.endBackupSub()
	}
}

func (c *Client) clearAutonomyIfDocked() {
	c.mu.Lock()
	had := c.autonomy || c.backupSub != nil || c.dockSession
	if had {
		c.autonomy = false
		c.dockSession = false
		c.dock = DockStatus{Code: DockSuccess, Label: "docked (charging)", Phase: "done"}
	}
	c.mu.Unlock()
	if had {
		c.endBackupSub()
		c.Log("info", "charging detected")
	}
}

func (c *Client) endBackupSub() {
	c.mu.Lock()
	sub := c.backupSub
	c.backupSub = nil
	c.mu.Unlock()
	if sub != nil {
		sub.Close()
	}
}

func (c *Client) publishTwist(x, y, az float64) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()
	if c.cmdPub == nil {
		return fmt.Errorf("Scout is offline")
	}
	c.cmdPub.Write(&geometry_msgs.Twist{
		Linear:  geometry_msgs.Vector3{X: x, Y: y, Z: 0},
		Angular: geometry_msgs.Vector3{X: 0, Y: 0, Z: az},
	})
	return nil
}

func (c *Client) Drive(x, y, az float64) error {
	if c.Tracked() {
		x = 0
	}
	moving := x != 0 || y != 0 || az != 0
	c.mu.Lock()
	was := c.wasDriving
	c.wasDriving = moving
	c.mu.Unlock()
	if moving {
		return c.publishTwist(x, y, az)
	}
	if was {
		return c.publishTwist(0, 0, 0)
	}
	return nil
}

func (c *Client) Stop() error {
	c.mu.Lock()
	c.wasDriving = false
	c.mu.Unlock()
	return c.publishTwist(0, 0, 0)
}

func (c *Client) ForceStop() error {
	c.Log("warn", "STOP")
	// Normal emergency stop is a zero velocity command. Dock cancellation is
	// separate; spawning its repeated cancel/service sequence on every Space
	// press was wasteful and could contend with manual control.
	return c.Stop()
}

func (c *Client) setVideoInts(pairs map[string]int32) error {
	ints := make([]IntParameter, 0, len(pairs))
	for name, value := range pairs {
		ints = append(ints, IntParameter{Name: name, Value: value})
	}
	cl, err := goroslib.NewServiceClient(goroslib.ServiceClientConf{
		Node: c.node, Name: "/ParamNode/video/set_parameters", Srv: &ReconfigureSrv{},
	})
	if err != nil {
		return err
	}
	defer cl.Close()
	var res ReconfigureRes
	if err := cl.Call(&ReconfigureReq{Config: DynConfig{Ints: ints}}, &res); err != nil {
		return err
	}
	for name, value := range pairs {
		switch name {
		case "image_night_mode":
			c.RememberVideoMode(value, -1)
		case "cameraLight":
			c.RememberVideoMode(-1, value)
		}
	}
	return nil
}

func (c *Client) SetNightMode(mode int32) error {
	if mode < NightModeColor || mode > NightModeAuto {
		return fmt.Errorf("invalid night mode %d", mode)
	}
	return c.setVideoInts(map[string]int32{"image_night_mode": mode})
}

func (c *Client) SetCameraLight(level int32) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	return c.setVideoInts(map[string]int32{"cameraLight": level})
}

func (c *Client) SetVideoMode(mode, light int32) error {
	if mode < NightModeColor || mode > NightModeAuto {
		return fmt.Errorf("invalid night mode %d", mode)
	}
	if light < 0 {
		light = 0
	}
	if light > 100 {
		light = 100
	}
	return c.setVideoInts(map[string]int32{
		"image_night_mode": mode,
		"cameraLight":      light,
	})
}

// RememberVideoMode caches ParamNode video ints.
func (c *Client) RememberVideoMode(nightMode, cameraLight int32) {
	c.mu.Lock()
	if nightMode >= 0 {
		c.nightMode = nightMode
	}
	if cameraLight >= 0 {
		c.cameraLight = cameraLight
	}
	c.mu.Unlock()
}

func (c *Client) VideoMode() (nightMode, cameraLight int32) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nightMode, c.cameraLight
}

func (c *Client) RefreshNight() {
	res, err := c.NightGet()
	if err != nil {
		if c.nightErr.CompareAndSwap(false, true) {
			c.Log("warn", "night_get: "+err.Error())
		}
		return
	}
	if c.nightErr.Swap(false) {
		c.Log("info", "night_get recovered")
	}
	c.mu.Lock()
	c.night = &res
	c.mu.Unlock()
}

func (c *Client) callNavCancel(name string) {
	cl, err := goroslib.NewServiceClient(goroslib.ServiceClientConf{
		Node: c.node, Name: name, Srv: &NavCancelSrv{},
	})
	if err != nil {
		return
	}
	defer cl.Close()
	_ = cl.Call(&NavCancelReq{}, &NavCancelRes{})
}

func (c *Client) publishCancelDetect() {
	if c.cancelDetectPub == nil {
		return
	}
	c.cancelDetectPub.Write(&std_msgs.Int32{Data: 1})
}

func (c *Client) CancelDock() error {
	c.mu.Lock()
	c.dockSession = false
	c.autonomy = false
	c.wasDriving = false
	c.dock = DockStatus{Code: DockCancel, Label: "idle", Phase: "idle"}
	c.mu.Unlock()
	c.endBackupSub()
	c.publishCancelDetect()
	c.publishTwist(0, 0, 0)
	go c.abortDockBackground()
	return nil
}

func (c *Client) abortDockBackground() {
	for i := 0; i < 15; i++ {
		c.publishCancelDetect()
		c.publishTwist(0, 0, 0)
		time.Sleep(80 * time.Millisecond)
	}
	go c.callNavCancel("/NavPathNode/nav_cancel")
	go c.callNavCancel("/CoreNode/nav_cancel")
}

func (c *Client) NightGet() (NightGetRes, error) {
	cl, err := goroslib.NewServiceClient(goroslib.ServiceClientConf{
		Node: c.node, Name: "/CoreNode/night_get", Srv: &NightGetSrv{},
	})
	if err != nil {
		return NightGetRes{}, err
	}
	defer cl.Close()
	var res NightGetRes
	err = cl.Call(&NightGetReq{}, &res)
	return res, err
}

func isKeyFrame(data []byte) bool {
	i := 0
	for i+4 < len(data) {
		var nalType byte
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			nalType = data[i+4] & 0x1f
			i += 4
		} else if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			nalType = data[i+3] & 0x1f
			i += 3
		} else {
			i++
			continue
		}
		if nalType == 5 || nalType == 7 || nalType == 8 {
			return true
		}
	}
	return false
}

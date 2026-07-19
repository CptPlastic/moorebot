package ros

import "github.com/bluenviron/goroslib/v2/pkg/msg"

// Status matches roller_eye/status (used by /SensorNode/simple_battery_status).
// Definitions must match status.msg exactly or the MD5 will not match publishers.
type Status struct {
	msg.Package     `ros:"roller_eye"`
	msg.Definitions `ros:"int8 PROCESS_OK=0,int8 PROCESS_ERROR=-1,int8 OBJ_DETECT_CHARGE=1,int8 RECORD_START=1,int8 RECORD_STOP=2,int8 RECORD_ERROR=3,int8 P2P_AV_PLAYING=1,int8 P2P_AV_STOP=2,int8 P2P_AV_ERROR=3,int8 WIFI_MODE_AP=0,int8 WIFI_MODE_STA=1,int8 WIFI_STATUS_DISCONNECT=0,int8 WIFI_STATUS_CONNECTED=1,int8 WIFI_STATUS_CONNECTING=2,int8 WIFI_STATUS_WRONG_KEY=3,int8 WIFI_STATUS_CONN_FAIL=4,int8 WIFI_STATUS_STOP=5,int8 BACK_UP_DETECT=1,int8 BACK_UP_ALIGN=2,int8 BACK_UP_BACK=3,int8 BACK_UP_SUCCESS=4,int8 BACK_UP_FAIL=5,int8 BACK_UP_INACTIVE=6,int8 BACK_UP_CANCEL=7,int8 BACK_UP_REDETECT=8,int8 BATTERY_CHARGING=0,int8 BATTERY_UNCHARGE=1,int8 BATTERY_FULL=2,int8 BATTERY_UNKOWN=3"`
	Status          []int32
}

const (
	BatteryCharging = 0
	BatteryUncharge = 1
	BatteryFull     = 2
	BatteryUnknown  = 3
)

type NavLowBatReq struct{}
type NavLowBatRes struct{}
type NavLowBatSrv struct {
	msg.Package `ros:"roller_eye"`
	Request     NavLowBatReq
	Response    NavLowBatRes
}

type NavCancelReq struct{}
type NavCancelRes struct{}
type NavCancelSrv struct {
	msg.Package `ros:"roller_eye"`
	Request     NavCancelReq
	Response    NavCancelRes
}

// Dock / going-home status values from roller_eye/status constants.
const (
	DockDetect   = 1
	DockAlign    = 2
	DockBackup   = 3
	DockSuccess  = 4
	DockFail     = 5
	DockInactive = 6
	DockCancel   = 7
)

type DockStatus struct {
	Code  int    `json:"code"`
	Label string `json:"label"`
	Phase string `json:"phase"` // idle | searching | docking | done | fail
}

type NightGetReq struct{}
type NightGetRes struct {
	// Scout uses camelCase field names (not ROS snake_case).
	IsNight    int8  `rosname:"isNight"`
	Brightness int32 `rosname:"brightness"`
}
type NightGetSrv struct {
	msg.Package `ros:"roller_eye"`
	Request     NightGetReq
	Response    NightGetRes
}

// dynamic_reconfigure/Reconfigure, used by /ParamNode/video/set_parameters.
// The ID tag matters: Go would otherwise turn ID into i_d and change the MD5.
type BoolParameter struct {
	Name  string
	Value bool
}
type IntParameter struct {
	Name  string
	Value int32
}
type StrParameter struct {
	Name  string
	Value string
}
type DoubleParameter struct {
	Name  string
	Value float64
}
type GroupState struct {
	Name   string
	State  bool
	ID     int32 `rosname:"id"`
	Parent int32
}
type DynConfig struct {
	Bools   []BoolParameter
	Ints    []IntParameter
	Strs    []StrParameter
	Doubles []DoubleParameter
	Groups  []GroupState
}
type ReconfigureReq struct {
	Config DynConfig
}
type ReconfigureRes struct {
	Config DynConfig
}
type ReconfigureSrv struct {
	msg.Package `ros:"dynamic_reconfigure"`
	Request     ReconfigureReq
	Response    ReconfigureRes
}

// NightMode values for /ParamNode/video/image_night_mode
const (
	NightModeColor = 0 // IR-cut in, color daylight
	NightModeIR    = 1 // night vision on
	NightModeAuto  = 2 // auto
)

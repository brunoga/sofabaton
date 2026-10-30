package wire

import "fmt"

// Opcode is the 16-bit opcode on the wire. Its high byte is the payload
// length and its low byte the family.
type Opcode uint16

// NewOpcode returns the opcode of a frame of family f with an n-byte
// payload.
func NewOpcode(f Family, n int) (Opcode, error) {
	if n < 0 || n > MaxPayload {
		return 0, fmt.Errorf("%w: %d bytes", ErrPayloadTooLong, n)
	}
	return Opcode(n)<<8 | Opcode(f), nil
}

// Family is the opcode's low byte.
func (o Opcode) Family() Family { return Family(o) }

// PayloadLen is the opcode's high byte.
func (o Opcode) PayloadLen() int { return int(o >> 8) }

func (o Opcode) String() string { return fmt.Sprintf("0x%04X", uint16(o)) }

// Direction tells which side sent a frame. Some families mean different
// things in each direction (0x05 is "leave learn mode" to the hub and
// "learn window closed" from it).
type Direction uint8

const (
	ToHub   Direction = iota + 1 // app or client to hub (A→H)
	FromHub                      // hub to app or client (H→A)
)

func (d Direction) String() string {
	switch d {
	case ToHub:
		return "A→H"
	case FromHub:
		return "H→A"
	}
	return fmt.Sprintf("Direction(%d)", uint8(d))
}

// Family is the opcode's low byte: what a frame is, independent of how long
// its payload is. The pages of one reply share a family.
type Family uint8

// Families sent to the hub. Names follow what the family does; see
// docs/protocol for what is known about each and how sure we are.
const (
	FamReqBanner          Family = 0x01
	FamLearnEnter         Family = 0x04 // arm IR learning
	FamLearnExit          Family = 0x05 // leave IR learning
	FamCreateDevice       Family = 0x07
	FamUpdateDevice       Family = 0x08
	FamDeleteEntity       Family = 0x09 // a device or, by id range, an activity
	FamReqDevices         Family = 0x0A
	FamReqBlob            Family = 0x0C
	FamWriteCommand       Family = 0x0E
	FamPlayBlob           Family = 0x0F
	FamDeleteKey          Family = 0x10 // a favorite or key row of an activity
	FamDeviceSort         Family = 0x11
	FamWriteMacro         Family = 0x12
	FamEraseConfiguration Family = 0x1D
	FamFindRemote         Family = 0x23
	FamReqRemoteList      Family = 0x2E // X2 paired remotes and repeaters
	FamSetHubName         Family = 0x30
	FamReqHubName         Family = 0x32
	FamCreateActivity     Family = 0x37
	FamWriteActivity      Family = 0x38
	FamReqActivities      Family = 0x3A
	FamReqButtons         Family = 0x3C
	FamWriteButton        Family = 0x3E
	FamActivate           Family = 0x3F
	FamReqIdleBehavior    Family = 0x40
	FamSetIdleBehavior    Family = 0x41
	FamWriteInputs        Family = 0x46
	FamReqActivityInputs  Family = 0x48
	FamReqMacros          Family = 0x4D
	FamConfirmMember      Family = 0x4F
	FamActivitySort       Family = 0x51
	FamReqVersion         Family = 0x58
	FamReqCommands        Family = 0x5C
	FamKeySort            Family = 0x61
	FamReqOrder           Family = 0x62
	FamRemoteSync         Family = 0x64
	FamAssignCommit       Family = 0x65
	FamReqActivityMap     Family = 0x6C
	FamRFLearn            Family = 0x70
	FamHubMQTTSetup       Family = 0x73
	FamHubMQTTStatus      Family = 0x74
	FamCallMe             Family = 0xC3 // UDP, both directions
)

// Families sent by the hub.
const (
	FamSaveAck          Family = 0x01
	FamBanner           Family = 0x02
	FamStatusAck        Family = 0x03 // payload[0] == 0 accepted, else rejected
	FamLearnExpired     Family = 0x05
	FamLearnData        Family = 0x06
	FamCreateDeviceAck  Family = 0x07 // payload[0] is the new device id
	FamDeviceRow        Family = 0x0B
	FamBlobPage         Family = 0x0D
	FamMacroAck         Family = 0x12
	FamMacroPage        Family = 0x13
	FamBatchResume      Family = 0x15
	FamRemoteStatus     Family = 0x2F
	FamHubNameReply     Family = 0x31
	FamCreateActAck     Family = 0x37 // payload[0] is the new activity id
	FamActivityRow      Family = 0x3B
	FamKeymapPage       Family = 0x3D
	FamButtonAck        Family = 0x3E // payload[0] echoes the button id
	FamIdleBehavior     Family = 0x42
	FamActivityInputs   Family = 0x47
	FamWiFiFirmware     Family = 0x59
	FamCommandPage      Family = 0x5D
	FamReady            Family = 0x60
	FamOrderReply       Family = 0x63
	FamOTAUpdate        Family = 0x67
	FamLearnFailed      Family = 0x68
	FamActivityMapPage  Family = 0x6D
	FamHubMQTTStatusAck Family = 0x74
	FamNotifyReply      Family = 0xC2 // UDP discovery reply
)

type familyKey struct {
	dir Direction
	fam Family
}

var familyNames = map[familyKey]string{
	{ToHub, FamReqBanner}:          "ReqBanner",
	{ToHub, FamLearnEnter}:         "LearnEnter",
	{ToHub, FamLearnExit}:          "LearnExit",
	{ToHub, FamCreateDevice}:       "CreateDevice",
	{ToHub, FamUpdateDevice}:       "UpdateDevice",
	{ToHub, FamDeleteEntity}:       "DeleteEntity",
	{ToHub, FamReqDevices}:         "ReqDevices",
	{ToHub, FamReqBlob}:            "ReqBlob",
	{ToHub, FamWriteCommand}:       "WriteCommand",
	{ToHub, FamPlayBlob}:           "PlayBlob",
	{ToHub, FamDeleteKey}:          "DeleteKey",
	{ToHub, FamDeviceSort}:         "DeviceSort",
	{ToHub, FamWriteMacro}:         "WriteMacro",
	{ToHub, FamEraseConfiguration}: "EraseConfiguration",
	{ToHub, FamFindRemote}:         "FindRemote",
	{ToHub, FamReqRemoteList}:      "ReqRemoteList",
	{ToHub, FamSetHubName}:         "SetHubName",
	{ToHub, FamReqHubName}:         "ReqHubName",
	{ToHub, FamCreateActivity}:     "CreateActivity",
	{ToHub, FamWriteActivity}:      "WriteActivity",
	{ToHub, FamReqActivities}:      "ReqActivities",
	{ToHub, FamReqButtons}:         "ReqButtons",
	{ToHub, FamWriteButton}:        "WriteButton",
	{ToHub, FamActivate}:           "Activate",
	{ToHub, FamReqIdleBehavior}:    "ReqIdleBehavior",
	{ToHub, FamSetIdleBehavior}:    "SetIdleBehavior",
	{ToHub, FamWriteInputs}:        "WriteInputs",
	{ToHub, FamReqActivityInputs}:  "ReqActivityInputs",
	{ToHub, FamReqMacros}:          "ReqMacros",
	{ToHub, FamConfirmMember}:      "ConfirmMember",
	{ToHub, FamActivitySort}:       "ActivitySort",
	{ToHub, FamReqVersion}:         "ReqVersion",
	{ToHub, FamReqCommands}:        "ReqCommands",
	{ToHub, FamKeySort}:            "KeySort",
	{ToHub, FamReqOrder}:           "ReqOrder",
	{ToHub, FamRemoteSync}:         "RemoteSync",
	{ToHub, FamAssignCommit}:       "AssignCommit",
	{ToHub, FamReqActivityMap}:     "ReqActivityMap",
	{ToHub, FamRFLearn}:            "RFLearn",
	{ToHub, FamHubMQTTSetup}:       "HubMQTTSetup",
	{ToHub, FamHubMQTTStatus}:      "HubMQTTStatus",
	{ToHub, FamCallMe}:             "CallMe",

	{FromHub, FamSaveAck}:          "SaveAck",
	{FromHub, FamBanner}:           "Banner",
	{FromHub, FamStatusAck}:        "StatusAck",
	{FromHub, FamLearnExpired}:     "LearnExpired",
	{FromHub, FamLearnData}:        "LearnData",
	{FromHub, FamCreateDeviceAck}:  "CreateDeviceAck",
	{FromHub, FamDeviceRow}:        "DeviceRow",
	{FromHub, FamBlobPage}:         "BlobPage",
	{FromHub, FamMacroAck}:         "MacroAck",
	{FromHub, FamMacroPage}:        "MacroPage",
	{FromHub, FamBatchResume}:      "BatchResume",
	{FromHub, FamRemoteStatus}:     "RemoteStatus",
	{FromHub, FamHubNameReply}:     "HubNameReply",
	{FromHub, FamCreateActAck}:     "CreateActivityAck",
	{FromHub, FamActivityRow}:      "ActivityRow",
	{FromHub, FamKeymapPage}:       "KeymapPage",
	{FromHub, FamButtonAck}:        "ButtonAck",
	{FromHub, FamIdleBehavior}:     "IdleBehavior",
	{FromHub, FamActivityInputs}:   "ActivityInputs",
	{FromHub, FamWiFiFirmware}:     "WiFiFirmware",
	{FromHub, FamCommandPage}:      "CommandPage",
	{FromHub, FamReady}:            "Ready",
	{FromHub, FamOrderReply}:       "OrderReply",
	{FromHub, FamOTAUpdate}:        "OTAUpdate",
	{FromHub, FamLearnFailed}:      "LearnFailed",
	{FromHub, FamActivityMapPage}:  "ActivityMapPage",
	{FromHub, FamHubMQTTStatusAck}: "HubMQTTStatus",
	{FromHub, FamNotifyReply}:      "NotifyReply",
	{FromHub, FamCallMe}:           "CallMe",
}

// Name is the family's name in direction d, or its hex value when the
// family is not known in that direction.
func (f Family) Name(d Direction) string {
	if n, ok := familyNames[familyKey{d, f}]; ok {
		return n
	}
	return f.String()
}

// Known reports whether the family has a meaning in direction d.
func (f Family) Known(d Direction) bool {
	_, ok := familyNames[familyKey{d, f}]
	return ok
}

func (f Family) String() string { return fmt.Sprintf("0x%02X", uint8(f)) }

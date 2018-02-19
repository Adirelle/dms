package upnp

import (
	"encoding/xml"
	"log"
	"reflect"
	"strings"

	"github.com/anacrolix/dms/logging"
)

const (
	ServiceControlPath   = "/control"
	SCPDPath             = "/scpd.xml"
	ServiceSubscribePath = "/subscribe"
)

// Service describes an UPNP Service
type Service struct {
	XMLName           xml.Name            `xml:"urn:schemas-upnp-org:service-1-0 scpd"`
	SpecVersion       specVersion         `xml:"specVersion"`
	ActionList        []actionDesc        `xml:"actionList>action"`
	ServiceStateTable []stateVariableDesc `xml:"serviceStateTable>stateVariable"`

	id      string
	urn     string
	logger  logging.Logger
	actions map[string]Action
	varMap  map[string]stateVariableDesc
}

type actionDesc struct {
	Name      string         `xml:"action"`
	Arguments []argumentDesc `xml:"argumentList>argument"`
}

type argumentDesc struct {
	Name            string `xml:"name"`
	Direction       string `xml:"direction"`
	RelatedStateVar string `xml:"relatedStateVariable"`
}

type stateVariableDesc struct {
	SendEvents    string    `xml:"sendEvents,attr"`
	Name          string    `xml:"name"`
	DataType      string    `xml:"dataType"`
	AllowedValues *[]string `xml:"allowedValueList>allowedValue,omitempty"`
}

// NewService initializes a new Service
func NewService(id, urn string, l logging.Logger) *Service {
	return &Service{
		id:          id,
		urn:         urn,
		logger:      l,
		actions:     make(map[string]Action),
		varMap:      make(map[string]stateVariableDesc),
		SpecVersion: specVersion{1, 0},
	}
}

// AddAction adds a new action the service specs.
// It panics if it already exists.
func (s *Service) AddAction(name string, action Action) {
	if _, exists := s.actions[name]; exists {
		log.Panicf("Action %q already defined", name)
	}
	s.actions[name] = action
	desc := actionDesc{Name: name}
	s.describeArgumentsFrom(&desc, "in", action.EmptyArguments())
	s.describeArgumentsFrom(&desc, "out", action.EmptyReturnValue())
	s.ActionList = append(s.ActionList, desc)
}

// AddActionFunc converts the given function to an action and adds it to the service.
// It panics if the action already exists or if the function cannot be converted (see soap.ActionFunc()).
func (s *Service) AddActionFunc(name string, f interface{}) {
	s.AddAction(name, ActionFunc(f))
}

func (s *Service) describeArgumentsFrom(desc *actionDesc, direction string, str interface{}) {
	refl := reflect.TypeOf(str)
	for i := 0; i < refl.NumField(); i++ {
		field := refl.Field(i)
		varName := s.describeStateVar(field)
		if varName != "" {
			desc.Arguments = append(desc.Arguments, argumentDesc{
				Name:            findArgName(field),
				Direction:       direction,
				RelatedStateVar: varName,
			})
		}
	}
}

func findArgName(f reflect.StructField) (name string) {
	name = f.Name
	if tag, ok := f.Tag.Lookup("xml"); ok {
		parts := strings.Split(tag, ",")
		if parts[0] != "" {
			name = parts[0]
		}
	}
	return
}

var upnpTypeMap = map[string]string{
	"uint8":     "ui1",
	"uint16":    "ui2",
	"uint32":    "ui4",
	"int8":      "i1",
	"int16":     "i2",
	"int32":     "i4",
	"int":       "i4",
	"float32":   "r4",
	"float64":   "r8",
	"bool":      "boolean",
	"string":    "string",
	"time.Time": "dateTime",
	"url.URL":   "uri",
	"uuid.UUID": "uuid",
	"[]uint8":   "bin",
}

func (s *Service) describeStateVar(f reflect.StructField) (name string) {
	name = f.Name
	parts := []string{"", "", ""}
	if tag, ok := f.Tag.Lookup("statevar"); ok {
		parts = append(strings.SplitN(tag, ",", 3), parts...)
	}
	if parts[0] != "" {
		name = parts[0]
	}
	if ignoreField(name, f.Type) {
		name = ""
		return
	}
	if _, exists := s.varMap[name]; exists {
		return
	}
	stateVar := stateVariableDesc{Name: name, SendEvents: "no"}
	if parts[1] != "" {
		stateVar.DataType = parts[1]
	} else if dt, ok := upnpTypeMap[f.Type.String()]; ok {
		stateVar.DataType = dt
	} else {
		log.Panicf("cannot map type of field %s: %s", f.Name, f.Type.String())
	}
	if parts[2] != "" {
		values := strings.Split(parts[2], ",")
		stateVar.AllowedValues = &values
	}
	s.varMap[name] = stateVar
	s.ServiceStateTable = append(s.ServiceStateTable, stateVar)
	return
}

func ignoreField(name string, type_ reflect.Type) bool {
	return name == "XMLName" || name == "XMLNS" || type_.String() == "xml.Name"
}

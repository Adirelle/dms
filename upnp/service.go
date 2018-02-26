package upnp

import (
	"encoding/xml"
	"fmt"
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
	Name      string         `xml:"name"`
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
func NewService(id, urn string) *Service {
	return &Service{
		id:          id,
		urn:         urn,
		actions:     make(map[string]Action),
		varMap:      make(map[string]stateVariableDesc),
		SpecVersion: specVersion{1, 0},
	}
}

// AddAction adds a new action the service specs.
func (s *Service) AddAction(name string, action Action) (err error) {
	if _, exists := s.actions[name]; exists {
		return fmt.Errorf("action %q already defined", name)
	}
	s.actions[name] = action
	desc := actionDesc{Name: name}
	err = s.describeArgumentsFrom(&desc, "in", action.EmptyArguments())
	if err != nil {
		return
	}
	err = s.describeArgumentsFrom(&desc, "out", action.EmptyReturnValue())
	if err != nil {
		return
	}
	s.ActionList = append(s.ActionList, desc)
	return nil
}

// AddActionFunc converts the given function to an action and adds it to the service.
func (s *Service) AddActionFunc(name string, f interface{}) (err error) {
	a, err := ActionFunc(f)
	if err == nil {
		err = s.AddAction(name, a)
	}
	return
}

func (s *Service) describeArgumentsFrom(desc *actionDesc, direction string, str interface{}) error {
	refl := reflect.TypeOf(str)
	for i := 0; i < refl.NumField(); i++ {
		field := refl.Field(i)
		varName, err := s.describeStateVar(field)
		if err != nil {
			return err
		}
		if varName != "" {
			desc.Arguments = append(desc.Arguments, argumentDesc{
				Name:            findArgName(field),
				Direction:       direction,
				RelatedStateVar: varName,
			})
		}
	}
	return nil
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

func (s *Service) describeStateVar(f reflect.StructField) (name string, err error) {
	name = f.Name
	parts := []string{"", "", ""}
	if tag, ok := f.Tag.Lookup("statevar"); ok {
		parts = append(strings.SplitN(tag, ",", 3), parts...)
	}
	if parts[0] != "" {
		name = parts[0]
	}
	if ignoreField(name, f) {
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
		err = fmt.Errorf("cannot map type of field %s: %s", f.Name, f.Type.String())
		return
	}
	if parts[2] != "" {
		values := strings.Split(parts[2], ",")
		stateVar.AllowedValues = &values
	}
	s.varMap[name] = stateVar
	s.ServiceStateTable = append(s.ServiceStateTable, stateVar)
	return
}

func ignoreField(name string, f reflect.StructField) bool {
	return name == "-" || name == "XMLName" || name == "XMLNS" || f.PkgPath != "" || f.Type.String() == "xml.Name"
}

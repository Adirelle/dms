package upnp

import (
	"encoding/xml"
	"fmt"
)

type Error struct {
	XMLName xml.Name `xml:"urn:schemas-upnp-org:control-1-0 UPnPError"`
	Code    uint     `xml:"errorCode"`
	Desc    string   `xml:"errorDescription"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d %s", e.Code, e.Desc)
}

const (
	InvalidActionErrorCode        = 401
	ActionFailedErrorCode         = 501
	ArgumentValueInvalidErrorCode = 600
)

var (
	InvalidActionError        = Errorf(InvalidActionErrorCode, "Invalid Action")
	ArgumentValueInvalidError = Errorf(ArgumentValueInvalidErrorCode, "The argument value is invalid")
)

// Errorf creates an UPNP error from the given code and description
func Errorf(code uint, tpl string, args ...interface{}) *Error {
	return &Error{Code: code, Desc: fmt.Sprintf(tpl, args...)}
}

// ConvertError converts any error to an UPNP error
func ConvertError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return Errorf(ActionFailedErrorCode, err.Error())
}

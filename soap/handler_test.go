package soap

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/anacrolix/dms/logging"
)

var sampleReq = `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" soapenv:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
	<soapenv:Header></soapenv:Header>
	<soapenv:Body>
		<a:args xmlns:a="http://www.example.com/ns/">
			<a:string>value</a:string>
			<a:number>6</a:number>
		</a:args>
	</soapenv:Body>
</soapenv:Envelope>`

var expectedResp = `<?xml version="1.0" encoding="UTF-8"?><Envelope xmlns="http://schemas.xmlsoap.org/soap/envelope/" encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><Body><rep xmlns="http://www.example.com/ns/"><fileList><file>foo</file><file>bar</file></fileList></rep></Body></Envelope>`

type TestArguments struct {
	XMLName xml.Name `xml:"http://www.example.com/ns/ args"`
	Str     string   `xml:"http://www.example.com/ns/ string"`
	Num     int      `xml:"http://www.example.com/ns/ number"`
}

type TestReply struct {
	XMLName xml.Name `xml:"http://www.example.com/ns/ reply"`
	Files   []string `xml:"fileList>file"`
}

type TestAction struct {
	t *testing.T
}

func (TestAction) Name() string {
	return "MyAction"
}

func (TestAction) EmptyArguments() interface{} {
	return TestArguments{}
}

func (a TestAction) Handle(args interface{}, r *http.Request) (interface{}, error) {
	a.t.Logf("arguments=%#v", args)
	ta := args.(TestArguments)
	if ta.Num != 6 || ta.Str != "value" {
		a.t.Errorf("arguments mismatch")
	}
	return TestReply{Files: []string{"foo", "bar"}}, nil
}

func TestHandler(t *testing.T) {
	logger := logging.NewTesting(t)
	action := TestAction{t}

	h := http.Header{}
	h.Set("SoapAction", `"MyAction"`)
	r := &http.Request{Method: "POST", Header: h, Body: ioutil.NopCloser(bytes.NewBufferString(sampleReq))}
	r = logging.RequestWithLogger(r, logger)

	srv := New(logger)
	srv.RegisterAction(action)

	w := &rwMock{t: t, status: http.StatusOK, header: http.Header{}}
	srv.ServeHTTP(w, r)

	t.Logf("output: %s", w.buf.String())

	if w.buf.String() != expectedResp {
		t.Fatalf("response mismatch")
	}

	t.Fatal("fail !")
}

type rwMock struct {
	header http.Header
	t      *testing.T
	status int
	buf    bytes.Buffer
}

func (m *rwMock) Header() http.Header {
	return m.header
}

func (m *rwMock) Write(b []byte) (int, error) {
	return m.buf.Write(b)
}

func (m *rwMock) WriteHeader(status int) {
	m.t.Logf("Status: %d", status)
	m.status = status
}

func ActionFuncToTest(args TestArguments, r *http.Request) (TestReply, error) {
	if args.Num != 6 || args.Str != "value" {
		panic("arguments mismatch")
	}
	return TestReply{Files: []string{"foo", "bar"}}, nil
}

func TestActionFunc(t *testing.T) {
	a := ActionFunc("MyAction", ActionFuncToTest)
	t.Logf("name=%s", a.Name())
	t.Logf("emptyArg=%#v", a.EmptyArguments())

	if a.Name() != "MyAction" {
		t.Error("Action name mismatch")
	}
	args := a.EmptyArguments()
	if _, ok := args.(TestArguments); !ok {
		t.Error("Argument type mismatch")
	}

	res, err := a.Handle(TestArguments{Num: 6, Str: "value"}, nil)
	t.Logf("res=%#v", res)
	t.Logf("err=%v", err)

	if _, ok := res.(TestReply); !ok {
		t.Error("Return value mismatch")
	}
}

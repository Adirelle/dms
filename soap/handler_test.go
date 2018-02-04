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
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
	<soapenv:Header></soapenv:Header>
	<soapenv:Body>
		<a:args xmlns:a="http://www.example.com/ns/">
			<a:string>value</a:string>
			<a:number>6</a:number>
		</a:args>
	</soapenv:Body>
</soapenv:Envelope>`

var expectedResp = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" +
	"<soapenv:Envelope xmlns:soapenv=\"http://schemas.xmlsoap.org/soap/envelope/\">" +
	"<soapenv:Body><rep xmlns=\"http://www.example.com/ns/\">" +
	"<fileList><file>foo</file><file>bar</file></fileList>" +
	"</rep></soapenv:Body></soapenv:Envelope>"

type Arguments struct {
	XMLName xml.Name `xml:"http://www.example.com/ns/ args"`
	Str     string   `xml:"string"`
	Num     int      `xml:"number"`
}

type Reply struct {
	XMLName xml.Name `xml:"http://www.example.com/ns/ rep"`
	Files   []string `xml:"fileList>file"`
}

func TestHandler(t *testing.T) {

	h := http.Header{}
	h.Set("SoapAction", "MyAction")
	r := &http.Request{Method: "POST", Header: h, Body: ioutil.NopCloser(bytes.NewBufferString(sampleReq))}

	args := Arguments{}
	var err error

	srv := New(logging.NewTesting(t))
	srv.RegisterAction("MyAction", HandlerFunc(func(c Call) {
		err = c.DecodeArguments(&args)
		resp := Reply{Files: []string{"foo", "bar"}}
		c.EncodeResponse(resp)
	}))

	w := &rwMock{t: t, status: http.StatusOK, header: http.Header{}}
	srv.ServeHTTP(w, r)

	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if args.Num != 6 || args.Str != "value" {
		t.Fatalf("arguments mismatch: %#v", args)
	}
	if w.buf.String() != expectedResp {
		t.Fatalf("response mismatch: %q", w.buf.String())
	}
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

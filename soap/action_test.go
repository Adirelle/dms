package soap

import (
	"net/http"
	"testing"
)

func ActionFuncToTest(args TestArguments, r *http.Request) (TestReply, error) {
	if args.Num != 6 || args.Str != "value" {
		panic("arguments mismatch")
	}
	return TestReply{Files: []string{"foo", "bar"}}, nil
}

func TestActionFunc(t *testing.T) {
	a := ActionFunc(ActionFuncToTest)
	t.Logf("name=%s", a.Name())
	t.Logf("emptyArg=%#v", a.EmptyArguments())

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

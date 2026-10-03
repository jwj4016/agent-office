package main

import (
	"context"
	"testing"
)

func TestPingReturnsAndEmits(t *testing.T) {
	var gotName string
	var gotData []interface{}
	a := &App{emit: func(_ context.Context, name string, data ...interface{}) {
		gotName, gotData = name, data
	}}
	a.startup(context.Background())

	first := a.Ping("안녕")
	second := a.Ping("again")

	if first.Message != "안녕" || first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("unexpected results: %+v %+v", first, second)
	}
	if gotName != EventPing || len(gotData) != 1 || gotData[0].(PingResult) != second {
		t.Fatalf("event not emitted correctly: %q %+v", gotName, gotData)
	}
}

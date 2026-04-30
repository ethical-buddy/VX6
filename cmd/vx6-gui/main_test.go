package main

import (
	"net/url"
	"reflect"
	"testing"
)

func TestBuildServiceAddArgs(t *testing.T) {
	form := url.Values{
		"name":       {"admin"},
		"target":     {"127.0.0.1:22"},
		"hidden":     {"on"},
		"alias":      {"ghost-admin"},
		"profile":    {"balanced"},
		"intro_mode": {"manual"},
		"intro":      {"relay1, relay2"},
	}
	got, err := buildServiceAddArgs(form)
	if err != nil {
		t.Fatalf("build service args: %v", err)
	}
	want := []string{
		"service", "add",
		"--name", "admin",
		"--target", "127.0.0.1:22",
		"--hidden",
		"--alias", "ghost-admin",
		"--profile", "balanced",
		"--intro-mode", "manual",
		"--intro", "relay1",
		"--intro", "relay2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestBuildReceiveArgs(t *testing.T) {
	form := url.Values{
		"mode": {"allow-node"},
		"node": {"alice"},
	}
	got, err := buildReceiveArgs(form)
	if err != nil {
		t.Fatalf("build receive args: %v", err)
	}
	want := []string{"receive", "allow", "--node", "alice"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestBuildDHTGetArgsRequiresSelector(t *testing.T) {
	if _, err := buildDHTGetArgs(url.Values{}); err == nil {
		t.Fatal("expected selector error")
	}
}

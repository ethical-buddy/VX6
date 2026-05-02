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

func TestBrowserTargetToArgsStatus(t *testing.T) {
	title, args, canonical, err := browserTargetToArgs("vx6://status")
	if err != nil {
		t.Fatalf("resolve browser target: %v", err)
	}
	if title != "Status" {
		t.Fatalf("unexpected title %q", title)
	}
	if !reflect.DeepEqual(args, []string{"status"}) {
		t.Fatalf("unexpected args %#v", args)
	}
	if canonical != "vx6://status" {
		t.Fatalf("unexpected canonical target %q", canonical)
	}
}

func TestBrowserTargetToArgsService(t *testing.T) {
	title, args, canonical, err := browserTargetToArgs("vx6://service/alice.web")
	if err != nil {
		t.Fatalf("resolve browser target: %v", err)
	}
	if title != "DHT Lookup" {
		t.Fatalf("unexpected title %q", title)
	}
	want := []string{"debug", "dht-get", "--service", "alice.web"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected args %#v", args)
	}
	if canonical != "vx6://service/alice.web" {
		t.Fatalf("unexpected canonical target %q", canonical)
	}
}

func TestBrowserTargetToArgsRejectsUnknown(t *testing.T) {
	if _, _, _, err := browserTargetToArgs("vx6://unknown/page"); err == nil {
		t.Fatal("expected browser target error")
	}
}
